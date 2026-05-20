package streams

import (
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/smazurov/videonode/internal/events"
	"github.com/smazurov/videonode/internal/ffmpeg"
	"github.com/smazurov/videonode/internal/logging"
	"github.com/smazurov/videonode/internal/process"
	"github.com/smazurov/videonode/internal/recording"
)

// ProcessState represents the current state of a stream process.
type ProcessState string

// Process states for stream FFmpeg processes.
const (
	ProcessStateIdle     ProcessState = "idle"
	ProcessStateStarting ProcessState = "starting"
	ProcessStateRunning  ProcessState = "running"
	ProcessStateStopping ProcessState = "stopping"
	ProcessStateError    ProcessState = "error"
)

// ProcessInfo contains information about a stream process.
type ProcessInfo struct {
	StreamID     string
	State        ProcessState
	PID          int
	StartedAt    time.Time
	RestartCount int
	LastError    error
}

// StreamProcessManager manages FFmpeg processes for all streams.
type StreamProcessManager interface {
	Start(streamID string) error
	Stop(streamID string) error
	Restart(streamID string) error
	// RestartCanvas reconciles canvas ownership against the spec, then restarts the canvas.
	RestartCanvas(canvasID string) error
	// ReleaseCanvas stops the canvas, releases ownership of its sources, and starts each source standalone.
	ReleaseCanvas(canvasID string) error
	GetStatus(streamID string) (*ProcessInfo, error)
	StartAll() error
	StopAll()
	IsRunning(streamID string) bool
	IsCrashed(streamID string) bool
	// CaptureRawSnapshot looks up the vision pipe by source ID regardless of canvas ownership.
	CaptureRawSnapshot(sourceStreamID string) ([]byte, error)
	OwnedBy(sourceStreamID string) string
}

// visionPipe is one source stream's raw-frame vision pipe.
// OwnerStreamID is the producing process (canvas ID when canvas-owned; else source ID).
type visionPipe struct {
	ownerStreamID string
	reader        *os.File
	width         int
	height        int
	latest        []byte // latest complete NV12 frame
	mu            sync.RWMutex
}

// streamProcessManager wraps process.Pool with stream-specific behavior.
type streamProcessManager struct {
	pool            process.Pool
	store           Store
	processor       *processor
	canvasProcessor *canvasProcessor
	producerMgr     *ProducerManager
	native          *NativePipelineConfig
	eventBus        *events.Bus
	logger          logging.Logger
	crashedStreams  map[string]bool
	// visionPipes keyed by source stream ID; sidecar always looks up by source.
	visionPipes map[string]*visionPipe
	// canvasOwnership: source ID → canvas ID currently owning its device.
	canvasOwnership map[string]string
	mu              sync.Mutex
}

// ProcessManagerOptions contains options for creating a StreamProcessManager.
type ProcessManagerOptions struct {
	Store           Store
	Processor       *processor
	CanvasProcessor *canvasProcessor
	EventBus        *events.Bus
	Native          *NativePipelineConfig
}

// NewStreamProcessManager creates a new StreamProcessManager.
func NewStreamProcessManager(opts *ProcessManagerOptions) StreamProcessManager {
	logger := logging.GetLogger("process_manager")

	spm := &streamProcessManager{
		store:           opts.Store,
		processor:       opts.Processor,
		canvasProcessor: opts.CanvasProcessor,
		native:          opts.Native,
		eventBus:        opts.EventBus,
		logger:          logger,
		crashedStreams:  make(map[string]bool),
		visionPipes:     make(map[string]*visionPipe),
		canvasOwnership: make(map[string]string),
	}

	spm.pool = process.NewPool(&process.PoolOptions{
		Logger:           logger,
		CommandProvider:  spm.generateCommand,
		OnStateChange:    spm.onStateChange,
		ConfigureProcess: spm.configureProcess,
	})

	// Producer processes (keyed "producer:<deviceID>") live in the same pool
	// but are owned by the ProducerManager. The canvasProcessor reads back
	// the per-device socket path from the manager when building sink cmds.
	spm.producerMgr = NewProducerManager(spm.pool)
	if spm.canvasProcessor != nil {
		spm.canvasProcessor.producerMgr = spm.producerMgr
	}

	return spm
}

// generateCommand generates the shell command for a pool process. Producer
// keys (producer:<deviceID>) delegate to ProducerManager; everything else is
// either a canvas stream or a single-camera stream.
func (m *streamProcessManager) generateCommand(streamID string) (string, error) {
	if IsProducerKey(streamID) {
		return m.producerMgr.Command(streamID)
	}

	config, exists := m.store.GetStream(streamID)
	if !exists {
		return "", fmt.Errorf("stream %s not found", streamID)
	}

	var processed *ProcessedStream
	var err error
	if config.Canvas != nil && m.canvasProcessor != nil {
		processed, err = m.canvasProcessor.processStream(streamID)
	} else {
		processed, err = m.processor.processStream(streamID)
	}
	if err != nil {
		return "", err
	}
	return processed.FFmpegCommand, nil
}

// onStateChange handles state transitions for event emission and crash recovery.
func (m *streamProcessManager) onStateChange(id string, _, newState process.State, _ error) {
	if newState == process.StateRunning {
		// Pool auto-restart doesn't go through Restart/RestartCanvas, so clear the crash flag here.
		m.mu.Lock()
		delete(m.crashedStreams, id)
		m.mu.Unlock()

		if m.eventBus != nil {
			m.eventBus.Publish(events.StreamStateChangedEvent{
				StreamID:  id,
				Enabled:   true,
				Timestamp: time.Now().Format(time.RFC3339),
			})
		}
	}

	if newState == process.StateError {
		m.mu.Lock()
		m.crashedStreams[id] = true
		// Drop vision pipes from the dead process; configureProcess reattaches on restart.
		m.clearVisionPipesByOwnerLocked(id)
		m.mu.Unlock()

		m.logger.Warn("Stream exited unexpectedly, restarting", "stream_id", id)

		if m.eventBus != nil {
			if streamConfig, exists := m.store.GetStream(id); exists && streamConfig.Device != "" {
				m.eventBus.Publish(events.StreamCrashedEvent{
					StreamID:  id,
					DeviceID:  streamConfig.Device,
					Timestamp: time.Now().Format(time.RFC3339),
				})
			}
		}

		go func() {
			if err := m.pool.Restart(id); err != nil {
				m.logger.Error("Failed to restart stream", "stream_id", id, "error", err)
			}
		}()
	}
}

// configureProcess sets up log parsing + vision pipe(s).
// Log channel is named after the kind of process so journald entries are
// filterable: producer:* → "videonode-source", canvas spec → "videonode-composer",
// single-stream native sink → "videonode-sink", everything else legacy
// → "ffmpeg".
func (m *streamProcessManager) configureProcess(streamID string, proc *process.Process) {
	proc.SetLogParser(
		logging.GetLogger(processLoggerName(streamID, m)).With("stream_id", streamID),
		ffmpeg.ParseLogLevel,
	)

	spec, exists := m.store.GetStream(streamID)
	if !exists {
		return
	}

	if spec.Canvas != nil {
		m.setupCanvasVisionPipes(streamID, spec.Canvas, proc)
		return
	}

	// Auto-enable vision when perspective is set (it needs the raw frame).
	visionEnabled := spec.Vision != nil && spec.Vision.Enabled
	if spec.Perspective != nil && !visionEnabled {
		visionEnabled = true
	}
	if !visionEnabled {
		return
	}

	w, h := visionDimensions(spec.Vision, spec.FFmpeg.Resolution)
	m.attachVisionPipe(streamID, streamID, proc, w, h)
}

// setupCanvasVisionPipes registers one pipe per canvas source, keyed by source ID, owned by canvasID.
func (m *streamProcessManager) setupCanvasVisionPipes(canvasID string, canvas *CanvasConfig, proc *process.Process) {
	for _, srcID := range canvas.SourceStreams {
		src, ok := m.store.GetStream(srcID)
		if !ok {
			continue
		}
		w, h := visionDimensions(src.Vision, src.FFmpeg.Resolution)
		m.attachVisionPipe(srcID, canvasID, proc, w, h)
	}
}

// attachVisionPipe creates a pipe, registers it keyed by sourceID, and spawns a reader goroutine.
func (m *streamProcessManager) attachVisionPipe(sourceID, ownerID string, proc *process.Process, w, h int) {
	reader, err := proc.SetupVisionPipe()
	if err != nil {
		m.logger.Error("Failed to setup vision pipe",
			"source_id", sourceID, "owner_id", ownerID, "error", err)
		return
	}

	vp := &visionPipe{
		ownerStreamID: ownerID,
		reader:        reader,
		width:         w,
		height:        h,
	}

	m.mu.Lock()
	m.visionPipes[sourceID] = vp
	m.mu.Unlock()

	go m.readVisionFrames(sourceID, vp)
}

// visionDimensions returns (W,H) for a vision pipe, defaulting to 640x480.
func visionDimensions(vc *ffmpeg.VisionConfig, inputResolution string) (int, int) {
	if vc != nil && vc.Width > 0 && vc.Height > 0 {
		return vc.Width, vc.Height
	}
	if w, h := parseResolutionWH(inputResolution); w > 0 && h > 0 {
		return w, h
	}
	return 640, 480
}

// resolveVisionFPS returns per-source FPS or default; 0 = no throttle, clamped to [0, 60].
func resolveVisionFPS(vc *ffmpeg.VisionConfig, defaultFPS int) int {
	fps := defaultFPS
	if vc != nil && vc.FPS > 0 {
		fps = vc.FPS
	}
	switch {
	case fps < 0:
		return 0
	case fps > 60:
		return 60
	}
	return fps
}

func parseResolutionWH(s string) (int, int) {
	w, h, _ := ffmpeg.ParseResolution(s)
	return w, h
}

// Start starts the FFmpeg process; canvas starts claim source ownership; owned individuals are rejected.
func (m *streamProcessManager) Start(streamID string) error {
	spec, exists := m.store.GetStream(streamID)
	if !exists {
		return fmt.Errorf("stream %s not found", streamID)
	}

	if spec.Canvas != nil {
		// Native path shares the producer via SCM_RIGHTS fanout — only stop
		// a running individual stream when it's on the legacy /dev/videoN path.
		nativeForCanvas := m.native.CanvasReady()
		for _, srcID := range spec.Canvas.SourceStreams {
			srcSpec, ok := m.store.GetStream(srcID)
			if !ok {
				continue
			}
			sourceShared := nativeForCanvas && m.shouldUseNativeForSingleStream(&srcSpec)
			if m.pool.IsRunning(srcID) && !sourceShared {
				if err := m.pool.Stop(srcID); err != nil {
					m.logger.Warn("Failed to stop source for canvas takeover",
						"canvas_id", streamID, "source_id", srcID, "error", err)
				}
			}
			m.mu.Lock()
			m.canvasOwnership[srcID] = streamID
			m.mu.Unlock()
		}
		// GPU canvases need an independently-supervised producer for each
		// source before the sink (composer | ffmpeg) starts. Legacy
		// (filter-graph) canvases don't use producers — they still spawn
		// ffmpeg with -i /dev/... directly via processor.processStream.
		// The choice is implicit: presence of the videonode-native binaries.
		if m.native.CanvasReady() {
			if err := m.acquireCanvasProducers(streamID, spec.Canvas); err != nil {
				// Roll back any partial acquires so refcounts don't leak.
				m.releaseCanvasProducers(streamID, spec.Canvas)
				return fmt.Errorf("canvas %s: producer acquire failed: %w", streamID, err)
			}
		}
		return m.pool.Start(streamID)
	}

	m.mu.Lock()
	owner, owned := m.canvasOwnership[streamID]
	m.mu.Unlock()
	if owned && !m.shouldUseNativeForSingleStream(&spec) {
		return fmt.Errorf("stream %s is owned by canvas %s — stop the canvas first", streamID, owner)
	}

	// Single V4L2 stream on the native path: acquire a producer for the
	// underlying device (refcounted; shared with any canvas pointing at
	// the same source). The legacy path (no native binaries / test mode /
	// custom command) skips this and ffmpeg opens /dev/videoN directly.
	if m.shouldUseNativeForSingleStream(&spec) {
		if err := m.acquireSingleStreamProducer(streamID, &spec); err != nil {
			return fmt.Errorf("stream %s: producer acquire failed: %w", streamID, err)
		}
	}
	return m.pool.Start(streamID)
}

// shouldUseNativeForSingleStream returns true when a non-canvas stream
// will route through processor.processStreamNative — i.e. binaries are
// installed, it's not a test stream, no custom command, and the device
// resolves to a real path.
func (m *streamProcessManager) shouldUseNativeForSingleStream(spec *StreamSpec) bool {
	if !m.native.SingleStreamReady() {
		return false
	}
	if spec.TestMode || spec.CustomFFmpegCommand != "" || spec.Device == "" {
		return false
	}
	resolver := m.deviceResolver()
	return resolver != nil && resolver(spec.Device) != ""
}

// acquireSingleStreamProducer wraps producerMgr.Acquire for the
// streamID-as-deviceID convention used by single streams.
func (m *streamProcessManager) acquireSingleStreamProducer(streamID string, spec *StreamSpec) error {
	if m.producerMgr == nil {
		return fmt.Errorf("no ProducerManager configured")
	}
	resolver := m.deviceResolver()
	devicePath := resolver(spec.Device)
	if devicePath == "" {
		return fmt.Errorf("device %q did not resolve to a path", spec.Device)
	}
	pspec := ProducerSpec{
		DeviceID:   streamID,
		DevicePath: devicePath,
		BinaryPath: m.native.V4L2Source,
	}
	if _, err := m.producerMgr.Acquire(pspec); err != nil {
		return err
	}
	m.logger.Info("Single-stream producer acquired",
		"stream_id", streamID, "device_path", devicePath)
	return nil
}

// acquireCanvasProducers Acquires one producer per source on the canvas.
// Caller is responsible for invoking releaseCanvasProducers on failure or
// teardown so refcounts stay balanced.
func (m *streamProcessManager) acquireCanvasProducers(canvasID string, canvas *CanvasConfig) error {
	if m.producerMgr == nil {
		return fmt.Errorf("no ProducerManager configured")
	}
	if m.native == nil || m.native.V4L2Source == "" {
		return fmt.Errorf("no videonode-source binary path configured")
	}
	resolver := m.deviceResolver()
	if resolver == nil {
		return fmt.Errorf("no deviceResolver configured")
	}
	for _, srcID := range canvas.SourceStreams {
		src, ok := m.store.GetStream(srcID)
		if !ok {
			return fmt.Errorf("source %q not found", srcID)
		}
		devicePath := resolver(src.Device)
		if devicePath == "" {
			return fmt.Errorf("source %q (%q) did not resolve to a device path", srcID, src.Device)
		}
		spec := ProducerSpec{
			DeviceID:   srcID,
			DevicePath: devicePath,
			BinaryPath: m.native.V4L2Source,
		}
		if _, err := m.producerMgr.Acquire(spec); err != nil {
			return fmt.Errorf("acquire producer for source %q: %w", srcID, err)
		}
	}
	m.logger.Info("Canvas producers acquired", "canvas_id", canvasID,
		"sources", canvas.SourceStreams)
	return nil
}

// releaseCanvasProducers releases the producer refcount for each source the
// canvas referenced. Safe to call multiple times — Release of an unknown
// device is a no-op.
func (m *streamProcessManager) releaseCanvasProducers(canvasID string, canvas *CanvasConfig) {
	if m.producerMgr == nil || canvas == nil {
		return
	}
	for _, srcID := range canvas.SourceStreams {
		m.producerMgr.Release(srcID)
	}
	m.logger.Info("Canvas producers released", "canvas_id", canvasID,
		"sources", canvas.SourceStreams)
}

// processLoggerName picks the journald log channel for a process based on
// its pool key and the stream spec it belongs to.
func processLoggerName(streamID string, m *streamProcessManager) string {
	if IsProducerKey(streamID) {
		return "videonode-source"
	}
	if m.store != nil {
		if spec, ok := m.store.GetStream(streamID); ok {
			if spec.Canvas != nil && m.native.CanvasReady() {
				return "videonode-composer"
			}
			if spec.Canvas == nil && spec.Device != "" && m.native.SingleStreamReady() {
				return "videonode-sink"
			}
		}
	}
	return "ffmpeg"
}

// deviceResolver pulls the resolver function from whichever processor is
// configured. Returned func may be nil if neither is wired (test harness).
func (m *streamProcessManager) deviceResolver() func(string) string {
	if m.canvasProcessor != nil && m.canvasProcessor.deviceResolver != nil {
		return m.canvasProcessor.deviceResolver
	}
	if m.processor != nil && m.processor.deviceResolver != nil {
		return m.processor.deviceResolver
	}
	return nil
}

// Stop gracefully stops the process; canvas Stop also clears source ownership
// and releases any per-source producers (GPU path only).
func (m *streamProcessManager) Stop(streamID string) error {
	// Read spec BEFORE pool.Stop so we know whether to release producers.
	spec, hadSpec := m.store.GetStream(streamID)

	err := m.pool.Stop(streamID)

	if hadSpec && spec.Canvas != nil && m.native.CanvasReady() {
		m.releaseCanvasProducers(streamID, spec.Canvas)
	} else if hadSpec && spec.Canvas == nil && m.shouldUseNativeForSingleStream(&spec) && m.producerMgr != nil {
		m.producerMgr.Release(streamID)
	}

	m.mu.Lock()
	m.clearVisionPipesByOwnerLocked(streamID)
	m.clearCanvasOwnershipLocked(streamID)
	m.mu.Unlock()

	return err
}

// Restart stops and restarts; no-op when canvas-owned (use RestartCanvas instead).
func (m *streamProcessManager) Restart(streamID string) error {
	var spec StreamSpec
	var hasSpec bool
	if m.store != nil {
		spec, hasSpec = m.store.GetStream(streamID)
	}
	m.mu.Lock()
	if owner, owned := m.canvasOwnership[streamID]; owned {
		// Native path allows shared producers — only block when on legacy.
		if !hasSpec || !m.shouldUseNativeForSingleStream(&spec) {
			m.mu.Unlock()
			m.logger.Debug("Restart skipped — stream owned by canvas",
				"stream_id", streamID, "canvas_id", owner)
			return nil
		}
	}
	delete(m.crashedStreams, streamID)
	m.clearVisionPipesByOwnerLocked(streamID)
	m.mu.Unlock()
	if !hasSpec {
		return m.pool.Restart(streamID)
	}
	if err := m.pool.Stop(streamID); err != nil {
		m.logger.Warn("Restart: pool.Stop failed", "stream_id", streamID, "error", err)
	}
	return m.Start(streamID)
}

// RestartCanvas reconciles ownership then restarts the canvas.
// Must stop canvas synchronously before starting released sources (v4l2 EBUSY race).
func (m *streamProcessManager) RestartCanvas(canvasID string) error {
	spec, exists := m.store.GetStream(canvasID)
	if !exists {
		return fmt.Errorf("stream %s not found", canvasID)
	}
	if spec.Canvas == nil {
		return fmt.Errorf("stream %s is not a canvas", canvasID)
	}

	wanted := make(map[string]bool, len(spec.Canvas.SourceStreams))
	for _, srcID := range spec.Canvas.SourceStreams {
		wanted[srcID] = true
	}

	var released, alreadyOwned []string
	m.mu.Lock()
	for srcID, owner := range m.canvasOwnership {
		if owner == canvasID {
			if !wanted[srcID] {
				delete(m.canvasOwnership, srcID)
				released = append(released, srcID)
			} else {
				alreadyOwned = append(alreadyOwned, srcID)
			}
		}
	}
	m.clearVisionPipesByOwnerLocked(canvasID)
	delete(m.crashedStreams, canvasID)
	m.mu.Unlock()

	if err := m.pool.Stop(canvasID); err != nil {
		m.logger.Warn("Failed to stop canvas before reconfigure",
			"canvas_id", canvasID, "error", err)
	}

	// For GPU canvases: release producers for dropped sources; acquire for
	// newly-added sources. Sources still in the new spec keep their refcount
	// (no producer churn).
	if m.native.CanvasReady() && m.producerMgr != nil {
		for _, srcID := range released {
			m.producerMgr.Release(srcID)
		}
		needAcquire := make([]string, 0, len(spec.Canvas.SourceStreams))
		owned := make(map[string]bool, len(alreadyOwned))
		for _, srcID := range alreadyOwned {
			owned[srcID] = true
		}
		for _, srcID := range spec.Canvas.SourceStreams {
			if !owned[srcID] {
				needAcquire = append(needAcquire, srcID)
			}
		}
		if len(needAcquire) > 0 {
			subset := &CanvasConfig{SourceStreams: needAcquire}
			if err := m.acquireCanvasProducers(canvasID, subset); err != nil {
				m.releaseCanvasProducers(canvasID, subset)
				return fmt.Errorf("canvas %s: producer acquire failed during restart: %w", canvasID, err)
			}
		}
	}

	for _, srcID := range released {
		if _, ok := m.store.GetStream(srcID); !ok {
			continue
		}
		if err := m.Start(srcID); err != nil {
			m.logger.Warn("Failed to restart released source after canvas drop",
				"canvas_id", canvasID, "source_id", srcID, "error", err)
		}
	}

	for _, srcID := range spec.Canvas.SourceStreams {
		if _, ok := m.store.GetStream(srcID); !ok {
			continue
		}
		if m.pool.IsRunning(srcID) {
			if err := m.pool.Stop(srcID); err != nil {
				m.logger.Warn("Failed to stop source for canvas takeover",
					"canvas_id", canvasID, "source_id", srcID, "error", err)
			}
		}
		m.mu.Lock()
		m.canvasOwnership[srcID] = canvasID
		m.mu.Unlock()
	}

	return m.pool.Start(canvasID)
}

// ReleaseCanvas stops the canvas process and starts each previously-owned source as a standalone stream.
// The canvas spec is left in the store; only runtime processes change. The canvas Stop happens
// synchronously before any source Start to avoid v4l2 EBUSY races, same rationale as RestartCanvas.
func (m *streamProcessManager) ReleaseCanvas(canvasID string) error {
	spec, exists := m.store.GetStream(canvasID)
	if !exists {
		return fmt.Errorf("stream %s not found", canvasID)
	}
	if spec.Canvas == nil {
		return fmt.Errorf("stream %s is not a canvas", canvasID)
	}

	m.mu.Lock()
	var released []string
	for srcID, owner := range m.canvasOwnership {
		if owner == canvasID {
			released = append(released, srcID)
		}
	}
	m.mu.Unlock()

	if err := m.pool.Stop(canvasID); err != nil {
		m.logger.Warn("Failed to stop canvas during release",
			"canvas_id", canvasID, "error", err)
	}

	m.mu.Lock()
	m.clearCanvasOwnershipLocked(canvasID)
	m.clearVisionPipesByOwnerLocked(canvasID)
	delete(m.crashedStreams, canvasID)
	m.mu.Unlock()

	for _, srcID := range released {
		if _, ok := m.store.GetStream(srcID); !ok {
			continue
		}
		if err := m.pool.Start(srcID); err != nil {
			m.logger.Warn("Failed to start released source after canvas release",
				"canvas_id", canvasID, "source_id", srcID, "error", err)
		}
	}

	return nil
}

// clearCanvasOwnershipLocked drops every claim held by canvasID. Caller holds m.mu.
func (m *streamProcessManager) clearCanvasOwnershipLocked(canvasID string) {
	for srcID, owner := range m.canvasOwnership {
		if owner == canvasID {
			delete(m.canvasOwnership, srcID)
		}
	}
}

// clearVisionPipesByOwnerLocked removes pipe entries produced by ownerID. Caller holds m.mu.
func (m *streamProcessManager) clearVisionPipesByOwnerLocked(ownerID string) {
	for srcID, vp := range m.visionPipes {
		if vp.ownerStreamID == ownerID {
			delete(m.visionPipes, srcID)
		}
	}
}

// IsCrashed returns true if the stream is in crashed state.
func (m *streamProcessManager) IsCrashed(streamID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.crashedStreams[streamID]
}

// OwnedBy returns the canvas ID owning the source, or "" if unowned.
func (m *streamProcessManager) OwnedBy(sourceStreamID string) string {
	m.mu.Lock()
	owner := m.canvasOwnership[sourceStreamID]
	m.mu.Unlock()
	if owner == "" {
		return ""
	}
	// Native fanout: if the source is independently running, it's not
	// "owned" — it shares the producer with the canvas.
	if m.pool.IsRunning(sourceStreamID) {
		return ""
	}
	return owner
}

// GetStatus returns the current state of a stream's process.
func (m *streamProcessManager) GetStatus(streamID string) (*ProcessInfo, error) {
	info := m.pool.GetStatus(streamID)
	return &ProcessInfo{
		StreamID:     info.ID,
		State:        ProcessState(info.State),
		StartedAt:    info.StartedAt,
		RestartCount: info.RestartCount,
		LastError:    info.LastError,
	}, nil
}

// IsRunning checks if a stream's process is currently running.
func (m *streamProcessManager) IsRunning(streamID string) bool {
	return m.pool.IsRunning(streamID)
}

// StartAll starts all streams; canvases first (claim ownership), then unowned individuals in parallel.
func (m *streamProcessManager) StartAll() error {
	allStreams := m.store.GetAllStreams()

	m.logger.Info("Starting all streams", "total_streams", len(allStreams))

	var canvasIDs, individualIDs []string
	for id, spec := range allStreams {
		switch {
		case spec.Canvas != nil && !spec.Canvas.IsEngaged():
			// Dormant canvas: skip startup so its sources can run standalone.
			m.logger.Info("Skipping dormant canvas on startup", "stream_id", id)
		case spec.Canvas != nil:
			canvasIDs = append(canvasIDs, id)
		default:
			individualIDs = append(individualIDs, id)
		}
	}

	var (
		errMu       sync.Mutex
		startErrors []error
	)
	recordErr := func(id string, err error) {
		m.logger.Error("Failed to start stream", "stream_id", id, "error", err)
		errMu.Lock()
		startErrors = append(startErrors, fmt.Errorf("stream %s: %w", id, err))
		errMu.Unlock()
	}

	// Canvases serially: parallel starts could race over a shared device claim.
	for _, id := range canvasIDs {
		if err := m.Start(id); err != nil {
			recordErr(id, err)
		}
	}

	var startWg sync.WaitGroup
	for _, id := range individualIDs {
		m.mu.Lock()
		_, owned := m.canvasOwnership[id]
		m.mu.Unlock()
		if owned {
			continue
		}
		startWg.Add(1)
		go func(streamID string) {
			defer startWg.Done()
			if err := m.Start(streamID); err != nil {
				recordErr(streamID, err)
			}
		}(id)
	}
	startWg.Wait()

	if len(startErrors) > 0 {
		return errors.Join(startErrors...)
	}
	return nil
}

// StopAll gracefully stops all running processes. Called on shutdown.
func (m *streamProcessManager) StopAll() {
	m.pool.StopAll()
}

// readVisionFrames reads NV12 frames into a double buffer, keeping only the latest.
func (m *streamProcessManager) readVisionFrames(sourceID string, vp *visionPipe) {
	frameSize := vp.width * vp.height * 3 / 2 // NV12: Y plane + UV plane

	bufs := [2][]byte{
		make([]byte, frameSize),
		make([]byte, frameSize),
	}
	idx := 0

	for {
		buf := bufs[idx]
		_, err := io.ReadFull(vp.reader, buf)
		if err != nil {
			m.logger.Debug("Vision pipe closed", "source_id", sourceID)
			break
		}
		vp.mu.Lock()
		vp.latest = buf
		vp.mu.Unlock()
		idx = 1 - idx
	}

	m.mu.Lock()
	// Only delete if entry is still this pipe object (avoid racing a new owner).
	if current, ok := m.visionPipes[sourceID]; ok && current == vp {
		delete(m.visionPipes, sourceID)
	}
	m.mu.Unlock()
}

// CaptureRawSnapshot returns a JPEG snapshot from the raw vision pipe.
func (m *streamProcessManager) CaptureRawSnapshot(streamID string) ([]byte, error) {
	// Snapshots are a source-level concept; canvases compose multiple sources.
	if spec, ok := m.store.GetStream(streamID); ok && spec.Canvas != nil {
		return nil, fmt.Errorf("%w: canvas %s — snapshot a source stream instead",
			recording.ErrSnapshotNotSupported, streamID)
	}

	// Native path: dial the producer's SCM_RIGHTS socket directly.
	if m.producerMgr != nil {
		if sock, ok := m.producerMgr.SocketPath(streamID); ok {
			if jpeg, err := captureNativeSnapshot(sock, 3*time.Second); err == nil {
				return jpeg, nil
			} else {
				m.logger.Debug("Native snapshot failed, falling back",
					"stream_id", streamID, "error", err)
			}
		}
	}

	// Legacy vision-pipe path (daemon-spawned ffmpeg with -filter_complex tap).
	m.mu.Lock()
	vp, ok := m.visionPipes[streamID]
	m.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("no vision pipe for stream %s", streamID)
	}

	vp.mu.RLock()
	if vp.latest == nil {
		vp.mu.RUnlock()
		return nil, fmt.Errorf("no frame available yet for stream %s", streamID)
	}
	frame := make([]byte, len(vp.latest))
	copy(frame, vp.latest)
	vp.mu.RUnlock()

	return ffmpeg.EncodeNV12ToJPEG(frame, vp.width, vp.height)
}
