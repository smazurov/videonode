package pipeline

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/smazurov/videonode/internal/events"
	"github.com/smazurov/videonode/internal/logging"
	"github.com/smazurov/videonode/internal/metrics/collectors"
	"github.com/smazurov/videonode/internal/process"
	"github.com/smazurov/videonode/internal/snapshots"
	"github.com/smazurov/videonode/internal/streams/pipelinectl"
)

// Config holds the daemon-wide knobs the Pipeline needs at construction
// time: paths to the three native binaries and the DRM render node.
type Config struct {
	VNSourceBin   string
	VNComposerBin string
	VNSinkBin     string
	DRMDevice     string

	// RTSPPort is the daemon's RTSP listen spec (e.g. ":8554" or
	// "10.0.0.1:8654"). buildEncoder hardcodes each encoder's single
	// output to the matching local RTSP relay URL; SRT and WebRTC fan out
	// from there. Empty falls back to the well-known ":8554".
	RTSPPort string

	// DeviceResolver maps an opaque device id (USB bus-port etc.) to a
	// canonical /dev/videoN path. Returns empty string when the device
	// can't be resolved; ApplySource surfaces that as an error. Optional
	// when every Source uses TestMode.
	DeviceResolver func(deviceID string) string

	// ControlServer, when non-nil, is used by ApplyComposer to register
	// the spawned videonode-composer over gRPC and push its initial
	// SetCanvas / SetSource / SetLayout / SetEffects RPCs. Without this
	// the composer starts but its canvas stays uninitialized and
	// downstream encoders see only black frames.
	ControlServer *pipelinectl.Manager

	// Registry, when non-nil, is used to Touch entities on pool state
	// transitions so their SSE snapshot refreshes with the new status.
	Registry *events.Registry

	// EventBus, when non-nil, receives the dedicated ProcessesEvent stream:
	// the current set of supervised processes is published on every pool
	// state transition and on each 2s stats sample while anything is
	// running. Separate from the per-entity Registry path above.
	EventBus *events.Bus

	// EncoderResolver maps a logical codec ("h264", "h265") and the
	// upstream pixel format ("nv12", "bgra", "") to the best validated
	// ffmpeg encoder + its HW-specific plumbing. Called by buildEncoder
	// when assembling an EncoderStage. When nil the pipeline falls back
	// to libx264/libx265.
	EncoderResolver func(codec, inputPixFmt string) (EncoderResolution, error)

	// EntityStore is the single source of truth for source/composer specs.
	// The pipeline reads through to it for every entity lookup (Apply,
	// RestartProcess, upstream resolution) instead of caching a second copy
	// that could drift from what was persisted. Required in production;
	// tests inject a fake.
	EntityStore EntityStore

	// StartedAtUS is the daemon start time (Unix microseconds), surfaced as
	// the "self" process row's uptime. When > 0 the pool samples the daemon's
	// own CPU/RSS and Snapshot() prepends a daemon row, which is what the
	// InfoBar rollup reads. Zero disables the self row.
	StartedAtUS int64
}

// EntityStore is the read surface the pipeline needs over persisted
// entities. The TOML store satisfies it structurally (streams.Source is
// an alias of pipeline.Source, likewise Composer), so no adapter is
// needed. Reads must be safe to call concurrently with writers.
type EntityStore interface {
	GetSourceEntity(id string) (Source, bool)
	GetComposerEntity(id string) (Composer, bool)
}

// EncoderResolution is the output of Config.EncoderResolver: the
// concrete ffmpeg encoder name plus any backend-specific args the
// encoder needs (e.g. -vaapi_device, hwupload filter chain).
type EncoderResolution struct {
	EncoderName  string
	GlobalArgs   []string
	VideoFilters string
}

// Pipeline is the stage assembler. One Pipeline owns the runtime state
// for all sources/composers/streams: per-entity Stage instances and the
// process.Pool that supervises every spawned binary. Entity specs are NOT
// cached here — the pipeline reads them through cfg.EntityStore (the single
// source of truth) so they can never drift from what was persisted.
//
// Public surface:
//   - ApplySource(s) / DeleteSource(id)
//   - ApplyComposer(c) / DeleteComposer(id)
//   - ApplyStream(s) / DeleteStream(id)
//
// Each Apply is idempotent — calling twice with the same spec is a no-op.
// Apply calls for the same id serialize via per-entity locks; calls for
// different ids run in parallel.
type Pipeline struct {
	cfg    Config
	pool   process.Pool
	logger logging.Logger

	mu         sync.Mutex
	stages     map[string]Stage                       // pool key → stage
	collectors map[string]*collectors.FFmpegCollector // stream id → progress collector

	entityLocksMu sync.Mutex
	entityLocks   map[string]*sync.Mutex
}

// New constructs a Pipeline. Logger is optional (defaults to a
// no-attr slog logger).
func New(cfg Config, logger logging.Logger) *Pipeline {
	if logger == nil {
		logger = logging.GetLogger("pipeline")
	}
	p := &Pipeline{
		cfg:         cfg,
		logger:      logger,
		stages:      make(map[string]Stage),
		collectors:  make(map[string]*collectors.FFmpegCollector),
		entityLocks: make(map[string]*sync.Mutex),
	}
	poolOpts := &process.PoolOptions{
		Logger:           logger,
		CommandProvider:  p.commandFor,
		ConfigureProcess: p.configureProcess,
		OnStateChange:    p.onStateChange,
		OnStats:          p.publishProcesses,
		OnRemove:         p.publishProcessRemoved,
	}
	if cfg.StartedAtUS > 0 {
		poolOpts.SelfSampler = &process.SelfSampler{}
		poolOpts.SelfStartedAtUS = cfg.StartedAtUS
	}
	p.pool = process.NewPool(poolOpts)
	return p
}

// ApplySource creates or updates the per-source `videonode-source`
// process. Validates that exactly one of Device or TestMode is set.
func (p *Pipeline) ApplySource(s Source) error {
	if s.ID == "" {
		return errors.New("pipeline: source.ID is required")
	}
	if s.TestMode && s.Device != "" {
		return fmt.Errorf("pipeline: source %s has both Device and TestMode set", s.ID)
	}
	if !s.TestMode && s.Device == "" {
		return fmt.Errorf("pipeline: source %s requires one of Device or TestMode", s.ID)
	}

	mu := p.entityLock("source:" + s.ID)
	mu.Lock()
	defer mu.Unlock()

	if err := p.ensureUdsDir(); err != nil {
		return fmt.Errorf("pipeline: mkdir uds dir: %w", err)
	}

	var devicePath string
	if !s.TestMode {
		if p.cfg.DeviceResolver == nil {
			return errors.New("pipeline: Config.DeviceResolver is nil")
		}
		// Unplugged devices resolve to their by-id path, not ""; the source
		// spawns and shows No Signal until the node reappears.
		devicePath = p.cfg.DeviceResolver(s.Device)
	}

	stage := &ProducerStage{
		SourceID:   s.ID,
		DevicePath: devicePath,
		TestMode:   s.TestMode,
		BinaryPath: p.cfg.VNSourceBin,
		GrpcUds:    GrpcSocketPathFor("source", s.ID),
	}
	p.replaceStage(stage)
	// Drop any prior control-plane registration so the next config push
	// re-dials the freshly-spawned UDS instead of holding a dead handle.
	if p.cfg.ControlServer != nil {
		p.cfg.ControlServer.Unregister(s.ID)
	}
	if err := p.restartStage(stage); err != nil {
		return err
	}
	// Initial gRPC registration + (when the spec carries one) SetFormat
	// push. Without registration the daemon can't reach the source over
	// gRPC at all, so SetFormat / Snapshot / status would all silently
	// fail. Mirrors the composer-side pushComposerConfig pattern.
	if p.cfg.ControlServer != nil {
		go p.registerAndConfigureSource(s, stage.GrpcUds)
	}
	return nil
}

// registerAndConfigureSource dials the source's gRPC UDS with a 30 s
// deadline (100 ms retry) so the source process has time to bind, then
// pushes the operator-selected V4L2 format if the spec carries one. Runs
// in its own goroutine; never blocks ApplySource.
func (p *Pipeline) registerAndConfigureSource(s Source, udsPath string) {
	sourceID := s.ID
	tag := []any{"source_id", sourceID, "uds", udsPath}

	const dialDeadline = 30 * time.Second
	const callTimeout = 5 * time.Second
	mgr := p.cfg.ControlServer

	deadline := time.Now().Add(dialDeadline)
	var lastErr error
	for {
		if time.Now().After(deadline) {
			p.logger.Warn("registerAndConfigureSource: register never succeeded",
				append(tag, "error", lastErr)...)
			return
		}
		regCtx, regCancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		err := mgr.RegisterSource(regCtx, sourceID, udsPath)
		regCancel()
		if err == nil {
			break
		}
		lastErr = err
		time.Sleep(100 * time.Millisecond)
	}

	if s.Format == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
	defer cancel()
	if _, err := mgr.SendSetFormat(ctx, sourceID, pipelinectl.SetFormatParams{
		FourCC: s.Format.FourCC,
		W:      s.Format.Width,
		H:      s.Format.Height,
		FPS:    s.Format.FPS,
	}); err != nil {
		p.logger.Warn("registerAndConfigureSource: initial SetFormat failed",
			append(tag, logging.KeyError, err)...)
		return
	}
	p.logger.Info("source initial format pushed",
		append(tag, "fourcc", s.Format.FourCC,
			"width", s.Format.Width, "height", s.Format.Height, "fps", s.Format.FPS)...)
}

// UpdateSourceFormat hot-applies a new V4L2 capture format to an already-
// registered source via the gRPC control plane. Does NOT restart the
// source process — connected SCM_RIGHTS consumers stay attached.
// Returns an error if the source isn't registered or the RPC fails;
// callers (source_service.Update) fall back to ApplySource on error.
func (p *Pipeline) UpdateSourceFormat(id string, f SourceFormat) error {
	if id == "" {
		return errors.New("pipeline: source.ID is required")
	}
	if p.cfg.ControlServer == nil {
		return errors.New("pipeline: ControlServer is nil; cannot hot-apply format")
	}
	if _, found := p.lookupSource(id); !found {
		return fmt.Errorf("pipeline: source %q not registered", id)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := p.cfg.ControlServer.SendSetFormat(ctx, id, pipelinectl.SetFormatParams{
		FourCC: f.FourCC,
		W:      f.Width,
		H:      f.Height,
		FPS:    f.FPS,
	}); err != nil {
		return err
	}
	return nil
}

// SourceLiveness reports the source's own health for the API `liveness`
// field, decoupled from the process `status` (pool state). Returns
// "offline" when the source process isn't running, the source binary's
// last-reported health token while running, or "unknown" when running
// but no status frame has arrived yet.
func (p *Pipeline) SourceLiveness(id string) string {
	if p.pool.GetStatus(SourcePoolKey(id)).State != process.StateRunning {
		return "offline"
	}
	if p.cfg.ControlServer != nil {
		if token, ok := p.cfg.ControlServer.SourceHealth(id); ok {
			return token
		}
	}
	return "unknown"
}

// SourceConsumerCount reports the live SCM_RIGHTS consumer count for a source,
// read from the sidecar's last status frame. Returns 0 when the source isn't
// running or no status has arrived yet. Lets REST reads seed the count, since
// the source.consumers SSE event only fires on membership change.
func (p *Pipeline) SourceConsumerCount(id string) int {
	if p.cfg.ControlServer == nil {
		return 0
	}
	if n, ok := p.cfg.ControlServer.SourceConsumerCount(id); ok {
		return n
	}
	return 0
}

// SourceColorMatrix reports the YCbCr matrix the source binary detected
// ("bt601"/"bt709"), read from its last status frame. Returns "" when the
// source isn't running or no status frame has reported a matrix yet.
func (p *Pipeline) SourceColorMatrix(id string) string {
	if p.cfg.ControlServer == nil {
		return ""
	}
	if m, ok := p.cfg.ControlServer.SourceColorMatrix(id); ok {
		return m
	}
	return ""
}

// StopSource stops the source's process and tears down its gRPC
// registration and stage. The entity stays in the store, so the UI still
// shows it as idle. Used by StopPipeline.
func (p *Pipeline) StopSource(id string) error {
	if id == "" {
		return nil
	}
	mu := p.entityLock("source:" + id)
	mu.Lock()
	defer mu.Unlock()

	if p.cfg.ControlServer != nil {
		p.cfg.ControlServer.Unregister(id)
	}
	poolID := SourcePoolKey(id)
	if err := p.pool.Stop(poolID); err != nil {
		p.logger.Warn("StopSource: pool.Stop failed", logging.KeyPoolID, poolID, logging.KeyError, err)
	}
	p.mu.Lock()
	delete(p.stages, poolID)
	p.mu.Unlock()
	return nil
}

// DeleteSource stops the source's `videonode-source` process and tears
// down its stage. No-op for unknown ids. The persisted entity is removed
// by the service layer. Callers are responsible
// for guaranteeing no composer or stream references this source — the
// service layer enforces that constraint.
func (p *Pipeline) DeleteSource(id string) error {
	if id == "" {
		return nil
	}
	mu := p.entityLock("source:" + id)
	mu.Lock()
	defer mu.Unlock()

	if p.cfg.ControlServer != nil {
		p.cfg.ControlServer.Unregister(id)
	}
	poolID := SourcePoolKey(id)
	if err := p.pool.Stop(poolID); err != nil {
		p.logger.Warn("DeleteSource: pool.Stop failed", logging.KeyPoolID, poolID, logging.KeyError, err)
	}
	p.mu.Lock()
	delete(p.stages, poolID)
	p.mu.Unlock()
	return nil
}

// ApplyComposer creates or updates the per-composer `videonode-composer`
// process. Per-frame layout/inputs/effects are pushed via the gRPC
// control plane post-spawn; this only manages the process lifecycle.
func (p *Pipeline) ApplyComposer(c Composer) error {
	if c.ID == "" {
		return errors.New("pipeline: composer.ID is required")
	}

	mu := p.entityLock("composer:" + c.ID)
	mu.Lock()
	defer mu.Unlock()

	if err := p.ensureUdsDir(); err != nil {
		return fmt.Errorf("pipeline: mkdir uds dir: %w", err)
	}

	canvasW, canvasH := c.Canvas.W, c.Canvas.H
	if canvasW <= 0 || canvasH <= 0 {
		canvasW, canvasH = 1920, 1080
	}
	stage := &ComposerStage{
		ComposerID: c.ID,
		BinaryPath: p.cfg.VNComposerBin,
		DRMDevice:  p.cfg.DRMDevice,
		CanvasFPS:  canvasFPSOrDefault(c.Canvas.FPS),
		CanvasW:    canvasW,
		CanvasH:    canvasH,
		GrpcUds:    GrpcSocketPathFor("composer", c.ID),
	}
	p.replaceStage(stage)
	// Drop any prior control-plane registration so the next config push
	// re-dials the freshly-spawned UDS instead of holding a dead handle.
	if p.cfg.ControlServer != nil {
		p.cfg.ControlServer.Unregister(c.ID)
	}
	if err := p.restartStage(stage); err != nil {
		return err
	}
	// Initial gRPC config push (SetCanvas/SetSource/SetLayout/SetEffects).
	// Without this the composer process runs but its canvas stays
	// uninitialized and every downstream encoder sees black frames.
	if p.cfg.ControlServer != nil {
		go p.pushComposerConfig(c, stage.GrpcUds)
	}
	return nil
}

// pushComposerConfig waits for the composer's gRPC UDS to come up,
// registers it with the control plane, then issues the initial
// SetCanvas / SetSource(per input) / SetLayout / SetEffects(per input
// with effect) RPCs. Runs in its own goroutine; bounded by a 30 s
// register deadline and a 5 s per-call timeout.
func (p *Pipeline) pushComposerConfig(c Composer, udsPath string) {
	composerID := c.ID
	tag := []any{"composer_id", composerID, "uds", udsPath}

	const dialDeadline = 30 * time.Second
	const callTimeout = 5 * time.Second
	mgr := p.cfg.ControlServer

	deadline := time.Now().Add(dialDeadline)
	var lastErr error
	for {
		if time.Now().After(deadline) {
			p.logger.Warn("pushComposerConfig: register never succeeded",
				append(tag, "error", lastErr)...)
			return
		}
		regCtx, regCancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		err := mgr.RegisterComposer(regCtx, composerID, udsPath)
		regCancel()
		if err == nil {
			break
		}
		lastErr = err
		time.Sleep(100 * time.Millisecond)
	}

	push := func(name string, fn func(context.Context) error) bool {
		ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
		defer cancel()
		if err := fn(ctx); err != nil {
			p.logger.Warn("pushComposerConfig: rpc failed",
				append(append([]any{}, tag...), "method", name, logging.KeyError, err)...)
			return false
		}
		return true
	}

	canvasW, canvasH := c.Canvas.W, c.Canvas.H
	if canvasW <= 0 || canvasH <= 0 {
		canvasW, canvasH = 1920, 1080
	}
	fps := uint32(canvasFPSOrDefault(c.Canvas.FPS))
	if !push("set_canvas", func(ctx context.Context) error {
		return mgr.SendSetCanvas(ctx, composerID, pipelinectl.SetCanvasParams{
			W: uint32(canvasW), H: uint32(canvasH), FPS: fps,
			BackgroundColor: c.Canvas.Background,
		})
	}) {
		return
	}

	// Bind each input to a slot. Slot label is positional ("a", "b", …)
	// to stay wire-compatible with the existing composer protocol.
	for i, in := range c.Inputs {
		slot := slotNameFor(i)
		sourceID := strings.TrimPrefix(in.Ref, "source:")
		if !push("set_source", func(ctx context.Context) error {
			return mgr.SendSetSource(ctx, composerID, pipelinectl.SetSourceParams{
				Slot:     slot,
				SourceID: sourceID,
				ScmPath:  SCMSocketPathFor(sourceID),
				Width:    uint32(canvasW),
				Height:   uint32(canvasH),
				FPS:      fps,
			})
		}) {
			return
		}
	}

	// SetLayout — map each LayoutSlot's Input ref to the matching input
	// index (slot label). When no layout is provided, fall back to a
	// single full-canvas slot referencing input 0.
	slots := make([]pipelinectl.LayoutSlotEntry, 0, len(c.Layout))
	if len(c.Layout) > 0 {
		inputIdx := make(map[string]int, len(c.Inputs))
		for i, in := range c.Inputs {
			inputIdx[in.Ref] = i
		}
		for _, l := range c.Layout {
			idx, ok := inputIdx[l.Input]
			if !ok {
				p.logger.Warn("pushComposerConfig: layout references unknown input",
					append(tag, "input", l.Input)...)
				continue
			}
			entry := pipelinectl.LayoutSlotEntry{
				Slot: slotNameFor(idx),
				X:    int32(l.X), Y: int32(l.Y), W: int32(l.W), H: int32(l.H),
				Rotation:        int32(l.Rotation),
				AspectRatioMode: aspectRatioModeToInt(l.AspectRatioMode),
			}
			if l.Crop != nil {
				entry.CropX = float32(l.Crop.X)
				entry.CropY = float32(l.Crop.Y)
				entry.CropScale = float32(l.Crop.Scale)
			}
			slots = append(slots, entry)
		}
	} else if len(c.Inputs) > 0 {
		slots = append(slots, pipelinectl.LayoutSlotEntry{
			Slot: slotNameFor(0), X: 0, Y: 0, W: int32(canvasW), H: int32(canvasH),
		})
	}
	if !push("set_layout", func(ctx context.Context) error {
		return mgr.SendSetLayout(ctx, composerID, pipelinectl.SetLayoutParams{Slots: slots})
	}) {
		return
	}

	// SetEffects per input that carries one.
	for _, in := range c.Inputs {
		if in.Effect == nil {
			continue
		}
		sourceID := strings.TrimPrefix(in.Ref, "source:")
		effect := *in.Effect
		if !push("set_effects", func(ctx context.Context) error {
			return mgr.SendSetEffects(ctx, composerID, pipelinectl.SetEffectsParams{
				SourceID: sourceID,
				Effects: []pipelinectl.EffectParams{{
					Type:           effect.Type,
					Corners:        effect.Corners,
					SnapshotWidth:  effect.SnapshotW,
					SnapshotHeight: effect.SnapshotH,
				}},
			})
		}) {
			return
		}
	}

	p.logger.Info("composer initial config pushed",
		append(tag, "canvas", fmt.Sprintf("%dx%d@%dfps", canvasW, canvasH, fps),
			"sources", len(c.Inputs))...)
}

// slotNameFor returns the alphabetic slot label used by the composer
// control plane ("a"..."z"; "slotN" past 26 for safety).
func aspectRatioModeToInt(s string) int32 {
	switch s {
	case "fit":
		return 1
	case "crop":
		return 2
	default:
		return 0
	}
}

func slotNameFor(i int) string {
	if i < 0 || i > 25 {
		return fmt.Sprintf("slot%d", i)
	}
	return string(rune('a' + i))
}

// UpdateComposerLayout hot-applies a new layout to a running composer
// via the gRPC control plane. Does NOT restart the composer process —
// callers use this for layout-only edits to avoid killing downstream
// vn-sink consumers. Returns an error if the composer isn't registered
// (e.g. spawn hasn't finished) or if any RPC fails.
func (p *Pipeline) UpdateComposerLayout(id string, layout []LayoutSlot) error {
	if id == "" {
		return errors.New("pipeline: composer.ID is required")
	}
	if p.cfg.ControlServer == nil {
		return errors.New("pipeline: ControlServer is nil; cannot hot-apply layout")
	}
	c, found := p.lookupComposer(id)
	if !found {
		return fmt.Errorf("pipeline: composer %q not registered: %w", id, pipelinectl.ErrNoSuchComposer)
	}
	inputIdx := make(map[string]int, len(c.Inputs))
	for i, in := range c.Inputs {
		inputIdx[in.Ref] = i
	}
	slots := make([]pipelinectl.LayoutSlotEntry, 0, len(layout))
	for _, l := range layout {
		idx, ok := inputIdx[l.Input]
		if !ok {
			return fmt.Errorf("pipeline: layout references unknown input %q", l.Input)
		}
		entry := pipelinectl.LayoutSlotEntry{
			Slot: slotNameFor(idx),
			X:    int32(l.X), Y: int32(l.Y), W: int32(l.W), H: int32(l.H),
			Rotation:        int32(l.Rotation),
			AspectRatioMode: aspectRatioModeToInt(l.AspectRatioMode),
		}
		if l.Crop != nil {
			entry.CropX = float32(l.Crop.X)
			entry.CropY = float32(l.Crop.Y)
			entry.CropScale = float32(l.Crop.Scale)
		}
		slots = append(slots, entry)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return p.cfg.ControlServer.SendSetLayout(ctx, id, pipelinectl.SetLayoutParams{Slots: slots})
}

// UpdateComposerCanvas hot-applies canvas parameters (dimensions, fps,
// background color) to a running composer via SetCanvas. Used for
// background-only edits, which the composer applies live without
// restarting — so downstream vn-sink consumers stay connected. The
// composer ignores mid-stream dimension changes (recreate is a STUB), so
// callers must restart via ApplyComposer for actual resizes.
func (p *Pipeline) UpdateComposerCanvas(id string, canvas CanvasDims) error {
	if id == "" {
		return errors.New("pipeline: composer.ID is required")
	}
	if p.cfg.ControlServer == nil {
		return errors.New("pipeline: ControlServer is nil; cannot hot-apply canvas")
	}
	if _, found := p.lookupComposer(id); !found {
		return fmt.Errorf("pipeline: composer %q not registered: %w", id, pipelinectl.ErrNoSuchComposer)
	}
	canvasW, canvasH := canvas.W, canvas.H
	if canvasW <= 0 || canvasH <= 0 {
		canvasW, canvasH = 1920, 1080
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return p.cfg.ControlServer.SendSetCanvas(ctx, id, pipelinectl.SetCanvasParams{
		W: uint32(canvasW), H: uint32(canvasH), FPS: uint32(canvasFPSOrDefault(canvas.FPS)),
		BackgroundColor: canvas.Background,
	})
}

// UpdateComposerEffect hot-applies an effect change for one input. A
// nil effect clears the input's effect list. Does NOT restart the
// composer process.
func (p *Pipeline) UpdateComposerEffect(id, inputRef string, effect *Effect) error {
	if id == "" {
		return errors.New("pipeline: composer.ID is required")
	}
	if p.cfg.ControlServer == nil {
		return errors.New("pipeline: ControlServer is nil; cannot hot-apply effect")
	}
	if _, found := p.lookupComposer(id); !found {
		return fmt.Errorf("pipeline: composer %q not registered: %w", id, pipelinectl.ErrNoSuchComposer)
	}
	sourceID := strings.TrimPrefix(inputRef, "source:")
	params := pipelinectl.SetEffectsParams{SourceID: sourceID}
	if effect != nil {
		params.Effects = []pipelinectl.EffectParams{{
			Type:           effect.Type,
			Corners:        effect.Corners,
			SnapshotWidth:  effect.SnapshotW,
			SnapshotHeight: effect.SnapshotH,
		}}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return p.cfg.ControlServer.SendSetEffects(ctx, id, params)
}

// StopComposer stops the composer's process and tears down its gRPC
// registration and stage. The entity stays in the store, so the UI still
// shows it as idle. Used by StopPipeline.
func (p *Pipeline) StopComposer(id string) error {
	if id == "" {
		return nil
	}
	mu := p.entityLock("composer:" + id)
	mu.Lock()
	defer mu.Unlock()

	if p.cfg.ControlServer != nil {
		p.cfg.ControlServer.Unregister(id)
	}
	poolID := ComposerPoolKey(id)
	if err := p.pool.Stop(poolID); err != nil {
		p.logger.Warn("StopComposer: pool.Stop failed", logging.KeyPoolID, poolID, logging.KeyError, err)
	}
	p.mu.Lock()
	delete(p.stages, poolID)
	p.mu.Unlock()
	return nil
}

// DeleteComposer stops the composer process and tears down its stage.
// No-op for unknown ids. The persisted entity is removed by the service.
func (p *Pipeline) DeleteComposer(id string) error {
	if id == "" {
		return nil
	}
	mu := p.entityLock("composer:" + id)
	mu.Lock()
	defer mu.Unlock()

	if p.cfg.ControlServer != nil {
		p.cfg.ControlServer.Unregister(id)
	}
	poolID := ComposerPoolKey(id)
	if err := p.pool.Stop(poolID); err != nil {
		p.logger.Warn("DeleteComposer: pool.Stop failed", logging.KeyPoolID, poolID, logging.KeyError, err)
	}
	p.mu.Lock()
	delete(p.stages, poolID)
	p.mu.Unlock()
	return nil
}

// ApplyStream creates or updates a per-stream encoder stage and caches it
// for the lazy-encoder lifecycle. The Upstream string is resolved against
// the source/composer registries:
//   - "source:<id>"   → ProducerFrameSource against the source's SCM
//   - "composer:<id>" → ComposerFrameSource against the composer's
//     SCM-out
//
// The encoder process is NOT spawned here. It stays idle until a reader
// connects and EnsureEncoder spawns it, and stops after the last reader
// disconnects. If the encoder is already running (a reader is attached
// while the spec changes) it is bounced so the new spec takes effect now.
//
// Dangling references (the named source/composer isn't registered) are
// an error.
func (p *Pipeline) ApplyStream(s Stream) error {
	if s.ID == "" {
		return errors.New("pipeline: stream.ID is required")
	}
	if s.Upstream == "" {
		return fmt.Errorf("pipeline: stream %s requires Upstream", s.ID)
	}

	mu := p.entityLock("stream:" + s.ID)
	mu.Lock()
	defer mu.Unlock()

	enc, err := p.buildEncoder(s)
	if err != nil {
		return fmt.Errorf("pipeline: build encoder %s: %w", s.ID, err)
	}
	p.replaceStage(enc)
	p.ensureCollector(s.ID)
	if !p.pool.IsRunning(EncoderIDFor(s.ID)) {
		return nil // idle: cached stage awaits the next reader-connect spawn
	}
	if err := p.restartStage(enc); err != nil {
		return fmt.Errorf("pipeline: restart encoder %s: %w", enc.ID(), err)
	}
	return nil
}

// RebuildStreamEncoder rebuilds a stream's encoder stage from the upstream's
// current dims and applies it WITHOUT violating the lazy-encoder lifecycle.
//
// The ffmpeg `-s WxH` is baked into the cached stage at build time
// (buildEncoder -> resolveUpstream), so when an upstream changes resolution
// (source SetFormat / composer SetCanvas) the cached stage must be rebuilt or
// the encoder keeps the stale geometry. If the encoder is currently running
// (a reader is attached) it is bounced so the new size takes effect now; if it
// is idle, only the cached stage is refreshed so the next reader-connect spawn
// (EnsureEncoder) picks up the new dims. Unlike ApplyStream, this never
// force-starts an idle encoder.
func (p *Pipeline) RebuildStreamEncoder(s Stream) error {
	if s.ID == "" {
		return errors.New("pipeline: stream.ID is required")
	}
	if s.Upstream == "" {
		return fmt.Errorf("pipeline: stream %s requires Upstream", s.ID)
	}

	mu := p.entityLock("stream:" + s.ID)
	mu.Lock()
	defer mu.Unlock()

	enc, err := p.buildEncoder(s)
	if err != nil {
		return fmt.Errorf("pipeline: build encoder %s: %w", s.ID, err)
	}
	p.replaceStage(enc)
	if !p.pool.IsRunning(EncoderIDFor(s.ID)) {
		return nil // idle: fresh stage cached for the next lazy spawn
	}
	if err := p.restartStage(enc); err != nil {
		return fmt.Errorf("pipeline: restart encoder %s: %w", enc.ID(), err)
	}
	return nil
}

// DeleteStream stops the encoder process and drops its cached stage.
// No-op for unknown ids. Upstream sources/composers stay warm.
func (p *Pipeline) DeleteStream(id string) error {
	if id == "" {
		return nil
	}
	mu := p.entityLock("stream:" + id)
	mu.Lock()
	defer mu.Unlock()

	p.stopCollector(id)
	poolID := EncoderIDFor(id)
	if err := p.pool.Stop(poolID); err != nil {
		p.logger.Warn("DeleteStream: pool.Stop failed", logging.KeyPoolID, poolID, logging.KeyError, err)
	}
	p.mu.Lock()
	delete(p.stages, poolID)
	p.mu.Unlock()
	return nil
}

// buildEncoder resolves the stream's Upstream string and constructs the
// matching EncoderStage. The Source/Composer must already be registered.
// The concrete ffmpeg encoder is chosen by Config.EncoderResolver from
// the stream's Codec + the upstream pixel format.
func (p *Pipeline) buildEncoder(s Stream) (*EncoderStage, error) {
	video, err := p.resolveUpstream(s.Upstream)
	if err != nil {
		return nil, err
	}

	resolved := p.resolveEncoder(s.Encoder.Codec, video)

	return &EncoderStage{
		OwnerStreamID:     s.ID,
		Media:             MediaSource{Video: video, Audio: ALSADirectAudio{Config: s.Audio}},
		Cfg:               s.Encoder,
		Resolved:          resolved,
		OutputURL:         localRTSPURL(p.cfg.RTSPPort, s.ID),
		CustomEncoderArgs: s.CustomEncoderArgs,
		VNSinkBin:         p.cfg.VNSinkBin,
	}, nil
}

// localRTSPURL builds the daemon's local RTSP relay URL for a stream id.
// The portSpec is the RTSP listen spec (":8554", "host:8554"); a bare
// ":port" resolves to localhost. This is the single hardcoded encoder
// output — SRT and WebRTC fan out from the in-memory stream this feeds.
func localRTSPURL(portSpec, streamID string) string {
	if portSpec == "" {
		portSpec = ":8554"
	}
	if portSpec[0] == ':' {
		portSpec = "localhost" + portSpec
	}
	return "rtsp://" + portSpec + "/" + streamID
}

// resolveEncoder calls the configured EncoderResolver or falls back to
// libx264/libx265 when no resolver is wired.
func (p *Pipeline) resolveEncoder(codec string, video FrameSource) EncoderResolution {
	inputPixFmt := ""
	if video != nil {
		inputPixFmt = "nv12"
	}

	if p.cfg.EncoderResolver != nil {
		res, err := p.cfg.EncoderResolver(codec, inputPixFmt)
		if err != nil {
			p.logger.Warn("EncoderResolver failed, using software fallback",
				logging.KeyCodec, codec, logging.KeyInputPixFmt, inputPixFmt, logging.KeyError, err)
		} else {
			p.logger.Info("Resolved encoder", logging.KeyCodec, codec, logging.KeyEncoder, res.EncoderName)
			return res
		}
	}

	fallback := "libx264"
	if codec == "h265" || codec == "hevc" {
		fallback = "libx265"
	}
	return EncoderResolution{EncoderName: fallback}
}

// lookupSource returns the persisted source spec from the single source of
// truth. The pipeline keeps no second copy, so this is the only way it
// learns a source's current shape.
func (p *Pipeline) lookupSource(id string) (Source, bool) {
	if p.cfg.EntityStore == nil {
		return Source{}, false
	}
	return p.cfg.EntityStore.GetSourceEntity(id)
}

// lookupComposer is the composer-side counterpart to lookupSource.
func (p *Pipeline) lookupComposer(id string) (Composer, bool) {
	if p.cfg.EntityStore == nil {
		return Composer{}, false
	}
	return p.cfg.EntityStore.GetComposerEntity(id)
}

// resolveUpstream maps a stream's Upstream reference ("source:<id>" or
// "composer:<id>") to a FrameSource that points at the right SCM socket.
func (p *Pipeline) resolveUpstream(upstream string) (FrameSource, error) {
	kind, id, ok := splitUpstream(upstream)
	if !ok {
		return nil, fmt.Errorf("pipeline: invalid upstream %q (want source:<id> or composer:<id>)", upstream)
	}
	switch kind {
	case "source":
		src, found := p.lookupSource(id)
		if !found {
			return nil, fmt.Errorf("pipeline: upstream source %q not registered", id)
		}
		pfs := ProducerFrameSource{Socket: SCMSocketPathFor(id)}
		if src.Format != nil {
			pfs.Width = int(src.Format.Width)
			pfs.Height = int(src.Format.Height)
			pfs.Fps = int(src.Format.FPS)
		}
		if pfs.Width == 0 || pfs.Height == 0 {
			pfs.Width, pfs.Height = 1920, 1080
		}
		if pfs.Fps == 0 {
			pfs.Fps = 30
		}
		// Prefer the source's detected matrix; until the first status frame
		// arrives, fall back to the SD/HD convention (height >= 720 → 709).
		pfs.ColorMatrix = "bt601"
		if pfs.Height >= 720 {
			pfs.ColorMatrix = "bt709"
		}
		if p.cfg.ControlServer != nil {
			if matrix, haveMatrix := p.cfg.ControlServer.SourceColorMatrix(id); haveMatrix {
				pfs.ColorMatrix = matrix
			}
		}
		return pfs, nil
	case "composer":
		c, found := p.lookupComposer(id)
		if !found {
			return nil, fmt.Errorf("pipeline: upstream composer %q not registered", id)
		}
		w := c.Canvas.W
		h := c.Canvas.H
		if w == 0 || h == 0 {
			w, h = 1920, 1080
		}
		sock := SCMSocketPathFor("composer-" + id)
		return ComposerFrameSource{Socket: sock, Width: w, Height: h, Fps: canvasFPSOrDefault(c.Canvas.FPS)}, nil
	default:
		return nil, fmt.Errorf("pipeline: unknown upstream kind %q", kind)
	}
}

// splitUpstream parses "source:foo" / "composer:bar" into (kind, id).
func splitUpstream(s string) (kind, id string, ok bool) {
	return SplitUpstream(s)
}

// SplitUpstream parses an upstream reference like "source:foo" or
// "composer:bar" into (kind, id, true). Returns ok=false for malformed
// input.
func SplitUpstream(s string) (kind, id string, ok bool) {
	idx := strings.IndexByte(s, ':')
	if idx <= 0 || idx == len(s)-1 {
		return "", "", false
	}
	return s[:idx], s[idx+1:], true
}

// StopEncoder stops the encoder stage for a stream if it is running.
// No-op when the encoder isn't running.
func (p *Pipeline) StopEncoder(streamID string) error {
	if streamID == "" {
		return errors.New("pipeline: streamID is required")
	}
	mu := p.entityLock("stream:" + streamID)
	mu.Lock()
	defer mu.Unlock()

	id := EncoderIDFor(streamID)
	if !p.pool.IsRunning(id) {
		return nil
	}
	if err := p.pool.Stop(id); err != nil {
		return fmt.Errorf("pipeline: stop encoder %s: %w", id, err)
	}
	return nil
}

// EnsureEncoder starts the encoder stage for a stream if it isn't
// already running. Requires ApplyStream to have been called previously.
func (p *Pipeline) EnsureEncoder(streamID string) error {
	if streamID == "" {
		return errors.New("pipeline: streamID is required")
	}
	mu := p.entityLock("stream:" + streamID)
	mu.Lock()
	defer mu.Unlock()

	id := EncoderIDFor(streamID)
	if p.pool.IsRunning(id) {
		return nil
	}
	p.mu.Lock()
	_, ok := p.stages[id]
	p.mu.Unlock()
	if !ok {
		return fmt.Errorf("pipeline: no cached encoder stage for stream %s (ApplyStream not called)", streamID)
	}
	if err := p.pool.Start(id); err != nil {
		return fmt.Errorf("pipeline: start encoder %s: %w", id, err)
	}
	return nil
}

// ErrNoSuchProcess is returned by RestartProcess when the pool id names a
// stage the pipeline isn't currently supervising. The API maps it to 404.
var ErrNoSuchProcess = errors.New("pipeline: no such supervised process")

// RestartProcess bounces a single supervised stage through the process
// pool, keyed by its pool id ("producer:<id>" / "composer:<id>" /
// "encoder:<stream-id>"). It is the one standardized restart entry point
// behind POST /api/processes/{id}/restart. Per kind:
//   - producer → re-ApplySource from the store (bounce + re-plumb the
//     gRPC control plane; revives a crashed source).
//   - composer → re-ApplyComposer from the store (bounce + re-push
//     canvas/source/layout/effects).
//   - encoder  → restartEncoder (pool.Restart unless idle; the lazy
//     lifecycle keeps an idle encoder down so we never spawn a
//     consumer-less process).
//
// Unknown / not-registered ids return ErrNoSuchProcess.
func (p *Pipeline) RestartProcess(poolID string) error {
	kind, id, ok := strings.Cut(poolID, ":")
	if !ok || id == "" {
		return ErrNoSuchProcess
	}
	switch kind {
	case "producer":
		src, found := p.lookupSource(id)
		if !found {
			return ErrNoSuchProcess
		}
		return p.ApplySource(src)
	case "composer":
		c, found := p.lookupComposer(id)
		if !found {
			return ErrNoSuchProcess
		}
		return p.ApplyComposer(c)
	case "encoder":
		p.mu.Lock()
		_, cached := p.stages[poolID]
		p.mu.Unlock()
		if !cached {
			return ErrNoSuchProcess
		}
		return p.restartEncoder(id)
	default:
		return ErrNoSuchProcess
	}
}

// restartEncoder bounces a stream's encoder process through the pool,
// reusing the cached stage. No-op when the encoder is idle (lazy
// lifecycle: no reader attached → no process, and force-starting would
// leave a consumer-less encoder running). A running or crashed encoder is
// restarted/revived. The FFmpegCollector and the upstream source/composer
// are unaffected — vn-sink retry-dials the upstream SCM.
func (p *Pipeline) restartEncoder(streamID string) error {
	if streamID == "" {
		return errors.New("pipeline: streamID is required")
	}
	mu := p.entityLock("stream:" + streamID)
	mu.Lock()
	defer mu.Unlock()

	id := EncoderIDFor(streamID)
	if p.pool.GetStatus(id).State == process.StateIdle {
		return nil
	}
	return p.pool.Restart(id)
}

// Pool exposes the underlying process.Pool for callers that need status
// lookups. Not used by Apply/Delete; only diagnostics should reach for
// it.
func (p *Pipeline) Pool() process.Pool { return p.pool }

// SnapshotSource dials the source's gRPC Snapshot RPC and returns the
// raw NV12 frame + metadata. The daemon's snapshot cache JPEG-encodes
// on demand; the Pipeline no longer ffmpeg-encodes inline.
func (p *Pipeline) SnapshotSource(ctx context.Context, sourceID string) (snapshots.Frame, error) {
	if p.cfg.ControlServer == nil {
		return snapshots.Frame{}, fmt.Errorf("no control server for snapshot")
	}
	resp, err := p.cfg.ControlServer.Snapshot(ctx, sourceID)
	if err != nil {
		return snapshots.Frame{}, err
	}
	cm, _ := p.cfg.ControlServer.SourceColorMatrix(sourceID)
	return snapshots.Frame{
		Bytes:       resp.GetNv12(),
		Format:      snapshots.FormatNV12,
		Width:       int(resp.GetWidth()),
		Height:      int(resp.GetHeight()),
		FrameIdx:    resp.GetFrameIdx(),
		CapturedNs:  resp.GetCapturedAtNs(),
		ColorMatrix: cm,
	}, nil
}

// SnapshotComposer dials the composer's gRPC Snapshot RPC and returns
// the raw NV12 canvas frame + metadata (symmetric with SnapshotSource).
func (p *Pipeline) SnapshotComposer(ctx context.Context, composerID string) (snapshots.Frame, error) {
	if p.cfg.ControlServer == nil {
		return snapshots.Frame{}, fmt.Errorf("no control server for snapshot")
	}
	resp, err := p.cfg.ControlServer.ComposerSnapshot(ctx, composerID)
	if err != nil {
		return snapshots.Frame{}, err
	}
	return snapshots.Frame{
		Bytes:       resp.GetNv12(),
		Format:      snapshots.FormatNV12,
		Width:       int(resp.GetWidth()),
		Height:      int(resp.GetHeight()),
		FrameIdx:    resp.GetFrameIdx(),
		CapturedNs:  resp.GetCapturedAtNs(),
		ColorMatrix: "bt709", // composer always outputs BT.709 limited
	}, nil
}

// ensureCollector starts an FFmpegCollector for the given stream if one
// isn't already running. The collector creates its Unix socket before
// returning, so it's ready to receive data when FFmpeg spawns.
func (p *Pipeline) ensureCollector(streamID string) {
	p.mu.Lock()
	_, exists := p.collectors[streamID]
	p.mu.Unlock()
	if exists {
		return
	}
	if err := p.ensureUdsDir(); err != nil {
		p.logger.Warn("ensureCollector: mkdir failed", logging.KeyStreamID, streamID, logging.KeyError, err)
		return
	}
	sockPath := ProgressSocketPathFor(streamID)
	c := collectors.NewFFmpegCollector(sockPath, streamID)
	if err := c.Start(context.Background()); err != nil {
		p.logger.Warn("ensureCollector: start failed", logging.KeyStreamID, streamID, logging.KeyError, err)
		return
	}
	p.mu.Lock()
	p.collectors[streamID] = c
	p.mu.Unlock()
}

// stopCollector stops and removes the FFmpegCollector for the given
// stream. Idempotent.
func (p *Pipeline) stopCollector(streamID string) {
	p.mu.Lock()
	c, ok := p.collectors[streamID]
	if ok {
		delete(p.collectors, streamID)
	}
	p.mu.Unlock()
	if ok {
		c.Stop()
	}
}

// ----- internal helpers -----

// entityLock returns a per-entity mutex, creating one on first use.
func (p *Pipeline) entityLock(key string) *sync.Mutex {
	p.entityLocksMu.Lock()
	defer p.entityLocksMu.Unlock()
	if mu, ok := p.entityLocks[key]; ok {
		return mu
	}
	mu := &sync.Mutex{}
	p.entityLocks[key] = mu
	return mu
}

// onStateChange touches the owning entity when a pool stage transitions so
// its SSE snapshot refreshes with the new pool-derived status field.
func (p *Pipeline) onStateChange(id string, _, newState process.State, _ error) {
	if p.cfg.Registry != nil {
		switch {
		case strings.HasPrefix(id, "producer:"):
			sourceID := strings.TrimPrefix(id, "producer:")
			p.cfg.Registry.Touch(context.Background(), "source", sourceID)
			if newState == process.StateIdle {
				// Full (zeroed) snapshot, never a partial: a partial status
				// payload would clobber the UI's last-known signal/broadcast.
				p.cfg.Registry.Publish("source", events.ActionStatus, sourceID, pipelinectl.StatusParams{
					DeviceID:    sourceID,
					Health:      "idle",
					TimestampMs: time.Now().UnixMilli(),
				})
			}
		case strings.HasPrefix(id, "composer:"):
			p.cfg.Registry.Touch(context.Background(), "composer", strings.TrimPrefix(id, "composer:"))
		case strings.HasPrefix(id, "encoder:"):
			p.cfg.Registry.Touch(context.Background(), "stream", strings.TrimPrefix(id, "encoder:"))
		}
	}
	// Push the whole supervised set on the dedicated process stream so the
	// transition reaches the operator UI immediately, independent of the
	// per-entity Touch above.
	p.publishProcesses()
}

// publishProcesses broadcasts the current supervised-process set on the
// dedicated ProcessesEvent stream. Wired as the pool's OnStats callback
// (fires per 2s stats sample while anything runs) and called from
// onStateChange (fires immediately on every transition). The payload is the
// same Snapshot() rows that back GET /api/processes — a cheap pool+stage
// join, not an entity recompute — so an idle pipeline stays silent because
// neither trigger fires when nothing is running. No-op when EventBus is nil.
func (p *Pipeline) publishProcesses() {
	if p.cfg.EventBus == nil {
		return
	}
	views := p.Snapshot()
	infos := make([]events.ProcessInfo, len(views))
	for i, v := range views {
		infos[i] = events.ProcessInfo{
			ID:           v.ID,
			Kind:         v.Kind,
			StreamID:     v.StreamID,
			State:        v.State,
			PID:          v.PID,
			StartedAtUS:  v.StartedAtUS,
			RestartCount: v.RestartCount,
			LastError:    v.LastError,
			RSSBytes:     v.RSSBytes,
			CPUPercent:   v.CPUPercent,
		}
	}
	events.Publish(p.cfg.EventBus, events.ProcessesEvent{
		Processes: infos,
		Timestamp: time.Now().Format(time.RFC3339),
	})
}

// publishProcessRemoved emits a ProcessRemovedEvent on the dedicated process
// stream. Wired as the pool's OnRemove callback so a deleted/stopped process
// is dropped from the operator UI immediately, even when nothing else is
// running to trigger the next stats sample. No-op when EventBus is nil.
func (p *Pipeline) publishProcessRemoved(id string) {
	if p.cfg.EventBus == nil {
		return
	}
	events.Publish(p.cfg.EventBus, events.ProcessRemovedEvent{
		ID:        id,
		Timestamp: time.Now().Format(time.RFC3339),
	})
}

// commandFor is the pool's CommandProvider callback.
func (p *Pipeline) commandFor(id string) (string, error) {
	p.mu.Lock()
	stage, ok := p.stages[id]
	p.mu.Unlock()
	if !ok {
		return "", fmt.Errorf("pipeline: no stage registered for pool id %s", id)
	}
	argv, _, err := stage.Command()
	if err != nil {
		return "", err
	}
	return shellJoinArgv(argv), nil
}

// configureProcess is the pool's Configurer callback. Wires the stage's
// LogParser + LogAttrs into the process.Process so each stage's stderr
// lands in journald with the right module + structured fields.
func (p *Pipeline) configureProcess(id string, proc *process.Process) {
	p.mu.Lock()
	stage, ok := p.stages[id]
	p.mu.Unlock()
	if !ok {
		return
	}
	moduleLogger := logging.GetLogger(stage.Kind().String())
	attrs := stage.LogAttrs()
	if len(attrs) > 0 {
		args := make([]any, len(attrs))
		for i, a := range attrs {
			args[i] = a
		}
		moduleLogger = moduleLogger.With(args...)
	}
	proc.SetLogParser(moduleLogger, stage.LogParser())
}

// replaceStage swaps the registered stage for an id. Caller follows
// with restartStage to spawn/respawn under the new spec.
func (p *Pipeline) replaceStage(stage Stage) {
	p.mu.Lock()
	p.stages[stage.ID()] = stage
	p.mu.Unlock()
}

func (p *Pipeline) restartStage(stage Stage) error {
	id := stage.ID()
	if p.pool.IsRunning(id) {
		if err := p.pool.Stop(id); err != nil {
			return err
		}
	}
	return p.pool.Start(id)
}

func (p *Pipeline) ensureUdsDir() error {
	return os.MkdirAll(NativeUdsDir, 0o755)
}

// canvasFPSOrDefault returns fps when positive, otherwise the daemon
// default canvas frame rate. Centralized so every composer-spawn /
// SetCanvas / downstream-encoder path agrees on the fallback.
func canvasFPSOrDefault(fps int) int {
	if fps > 0 {
		return fps
	}
	return DefaultCanvasFPS
}
