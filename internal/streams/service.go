package streams

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"sync"
	"time"

	"github.com/smazurov/videonode/internal/encoders"
	"github.com/smazurov/videonode/internal/events"
	"github.com/smazurov/videonode/internal/logging"
	"github.com/smazurov/videonode/internal/streams/pipelinectl"
	"github.com/smazurov/videonode/internal/types"
)

// onlyPerspectiveChanged reports whether the only difference between
// before and after is in the Perspective field. Used by UpdatePartial
// to skip a canvas restart when the live IPC push has already
// delivered the new corners to the composer.
func onlyPerspectiveChanged(before, after StreamSpec) bool {
	before.Perspective = nil
	after.Perspective = nil
	before.UpdatedAt = after.UpdatedAt
	return reflect.DeepEqual(before, after)
}

// ServiceOptions contains optional configuration for the stream service.
type ServiceOptions struct {
	Store            Store // required
	EncoderSelector  encoders.Selector
	EventBus         *events.Bus
	ProcessManager   StreamProcessManager
	VisionDefaultFPS int                   // default FPS for vision pipes; 0 = no throttle
	Native           *NativePipelineConfig // optional; when binaries are present, single V4L2 streams + canvases take the dma-buf path
	// ControlServer is the daemon-wide pipelinectl server (single source
	// of truth for both the UDS path and the daemon→composer dispatch
	// surface). Nil disables the control plane — sources can't be
	// commanded and the GPU compose path renders black frames forever.
	// main.go owns the lifecycle.
	ControlServer *pipelinectl.Manager
	// RTSPPort is the host:port the daemon's embedded RTSP server is
	// listening on. The GPU compose path's ffmpeg sink targets it. Empty
	// = the well-known default "127.0.0.1:8554".
	RTSPPort string
}

type service struct {
	store              Store
	streams            map[string]*Stream
	streamsMutex       sync.RWMutex
	processManager     StreamProcessManager
	encoderSelector    encoders.Selector
	validationProvider types.ValidationProvider
	eventBus           *events.Bus
	deviceResolver     func(string) string
	rtspHost           string
	logger             logging.Logger
}

// NewStreamService creates a new stream service with options.
func NewStreamService(opts *ServiceOptions) StreamService {
	logger := logging.GetLogger("streams")

	if opts == nil {
		logger.Error("ServiceOptions is required")
		panic("ServiceOptions is required")
	}
	repo := opts.Store
	if repo == nil {
		logger.Error("Store is required in ServiceOptions")
		panic("Store is required in ServiceOptions")
	}

	encoderSelector := makeEncoderSelector(logger, opts, repo)
	rtspHost := resolveRTSPHost(opts.RTSPPort)
	deviceResolverFunc := makeDeviceResolver(logger)

	svc := &service{
		store:              repo,
		streams:            make(map[string]*Stream),
		encoderSelector:    encoderSelector,
		validationProvider: NewValidationService(repo),
		deviceResolver:     deviceResolverFunc,
		rtspHost:           rtspHost,
		logger:             logger,
	}
	svc.eventBus = opts.EventBus

	if opts.ProcessManager == nil {
		logger.Error("ServiceOptions.ProcessManager is required " +
			"(legacy auto-construction no longer supported)")
		panic("ServiceOptions.ProcessManager is required")
	}
	svc.processManager = opts.ProcessManager

	return svc
}

// CreateStream creates a new video stream (single-camera or canvas composite).
func (s *service) CreateStream(ctx context.Context, params StreamCreateParams) (*Stream, error) {
	if params.Canvas != nil {
		return s.createCanvasStream(ctx, params)
	}
	return s.createSingleStream(ctx, params)
}

// createSingleStream creates a single-camera stream.
func (s *service) createSingleStream(_ context.Context, params StreamCreateParams) (*Stream, error) {
	// Validate device ID using processor's device resolver
	devicePath := s.deviceResolver(params.DeviceID)
	if devicePath == "" {
		return nil, NewStreamError(ErrCodeDeviceNotFound,
			fmt.Sprintf("device %s not found or not available", params.DeviceID), nil)
	}

	// Use provided stream ID
	streamID := params.StreamID

	// Check if stream already exists
	_, exists := s.getStreamSafe(streamID)
	if exists {
		return nil, NewStreamError(ErrCodeStreamExists,
			fmt.Sprintf("stream %s already exists", streamID), nil)
	}

	// Build resolution and framerate using helpers
	resolution := formatResolution(params.Width, params.Height)
	fps := formatFPS(params.Framerate)

	// Validate and build stream configuration
	if err := validateCodec(params.Codec); err != nil {
		return nil, NewStreamError(ErrCodeInvalidParams, err.Error(), nil)
	}

	qualityParams := buildQualityParams(params.Bitrate)
	ffmpegOptions := buildFFmpegOptions(params.Options)

	streamConfigTOML := StreamSpec{
		ID:     streamID,
		Name:   streamID,
		Device: params.DeviceID,
		FFmpeg: FFmpegConfig{
			Codec:         params.Codec,
			InputFormat:   params.InputFormat,
			Resolution:    resolution,
			FPS:           fps,
			Options:       ffmpegOptions,      // Apply user-provided or default options
			QualityParams: qualityParams,      // Store quality params for future use
			AudioDevice:   params.AudioDevice, // Pass through audio device if specified
			Rotation:      params.Rotation,
		},
		CreatedAt: time.Now(),
	}

	// Initialize the stream with all integrations FIRST (so it's in memory)
	if err := s.InitializeStream(streamConfigTOML); err != nil {
		return nil, NewStreamError(ErrCodeMonitoringError,
			"failed to initialize stream", err)
	}

	// Set initial enabled state to true since device was validated as available
	s.streamsMutex.Lock()
	if stream, found := s.streams[streamID]; found {
		stream.Enabled = true
	}
	s.streamsMutex.Unlock()

	// Save to persistent TOML config
	if s.store != nil {
		if err := s.store.AddStream(streamConfigTOML); err != nil {
			s.logger.Warn("Failed to save stream to TOML config", "stream_id", streamID, "error", err)
		} else {
			s.logger.Info("Saved stream to persistent TOML config", "stream_id", streamID)

			// Start FFmpeg process via process manager
			if s.processManager != nil {
				if err := s.processManager.Start(streamID); err != nil {
					s.logger.Warn("Failed to start stream process", "stream_id", streamID, "error", err)
				}
			}
		}
	}

	// Get the created stream from memory
	stream, exists := s.getStreamSafe(streamID)
	if !exists {
		return nil, NewStreamError(ErrCodeStreamNotFound,
			fmt.Sprintf("stream %s was created but not found in memory", streamID), nil)
	}

	// Emit stream state changed event
	if s.eventBus != nil {
		s.eventBus.Publish(events.StreamStateChangedEvent{
			StreamID:  streamID,
			Enabled:   true,
			Timestamp: time.Now().Format(time.RFC3339),
		})
	}

	return copyStream(stream), nil
}

// autoDisableNewSources auto-flips Enabled=false on each source stream
// that just became a member of a canvas. The user can re-enable a source
// manually if they want it published standalone alongside the canvas.
func (s *service) autoDisableNewSources(ctx context.Context, addedSourceIDs []string) {
	for _, srcID := range addedSourceIDs {
		stream, ok := s.getStreamSafe(srcID)
		if !ok || !stream.Enabled {
			continue
		}
		if _, err := s.SetEnabled(ctx, srcID, false); err != nil {
			s.logger.Warn("autoDisableNewSources: SetEnabled failed",
				"source_id", srcID, "error", err)
		}
	}
}

// diffAddedSources returns members of next not present in prev.
func diffAddedSources(prev, next []string) []string {
	seen := make(map[string]struct{}, len(prev))
	for _, id := range prev {
		seen[id] = struct{}{}
	}
	added := make([]string, 0, len(next))
	for _, id := range next {
		if _, ok := seen[id]; !ok {
			added = append(added, id)
		}
	}
	return added
}

// syncCanvasInputsEnabledLocked syncs canvas.InputsEnabled to sources. Caller must hold streamsMutex.
func (s *service) syncCanvasInputsEnabledLocked(canvas *Stream, sources []string) {
	next := make(map[string]bool, len(sources))
	for _, srcID := range sources {
		if v, ok := canvas.InputsEnabled[srcID]; ok {
			next[srcID] = v
			continue
		}
		if src, ok := s.streams[srcID]; ok {
			next[srcID] = src.Enabled
		}
	}
	canvas.InputsEnabled = next
}

// validateCanvasConfig checks that a CanvasConfig is well-formed and references existing non-canvas sources.
func (s *service) validateCanvasConfig(canvas *CanvasConfig) error {
	if canvas == nil {
		return fmt.Errorf("canvas config is required")
	}

	validSize := (canvas.Width == 1920 && canvas.Height == 1080) ||
		(canvas.Width == 2560 && canvas.Height == 1440) ||
		(canvas.Width == 3840 && canvas.Height == 2160)
	if !validSize {
		return fmt.Errorf("canvas size must be 1920x1080, 2560x1440, or 3840x2160, got %dx%d",
			canvas.Width, canvas.Height)
	}
	if canvas.FPS == "" {
		return fmt.Errorf("canvas fps is required")
	}

	n := len(canvas.SourceStreams)
	if n < 1 || n > 4 {
		return fmt.Errorf("canvas must reference 1–4 source streams, got %d", n)
	}
	seen := make(map[string]bool, n)
	for _, srcID := range canvas.SourceStreams {
		if srcID == "" {
			return fmt.Errorf("source stream ID cannot be empty")
		}
		if seen[srcID] {
			return fmt.Errorf("duplicate source stream: %s", srcID)
		}
		seen[srcID] = true

		src, exists := s.store.GetStream(srcID)
		if !exists {
			return fmt.Errorf("source stream %s not found", srcID)
		}
		if src.Canvas != nil {
			return fmt.Errorf("source stream %s is itself a canvas (nesting not allowed)", srcID)
		}
	}

	return nil
}

// createCanvasStream creates a composite canvas stream that live-references existing streams.
func (s *service) createCanvasStream(_ context.Context, params StreamCreateParams) (*Stream, error) {
	streamID := params.StreamID

	if _, exists := s.getStreamSafe(streamID); exists {
		return nil, NewStreamError(ErrCodeStreamExists,
			fmt.Sprintf("stream %s already exists", streamID), nil)
	}

	if err := validateCodec(params.Codec); err != nil {
		return nil, NewStreamError(ErrCodeInvalidParams, err.Error(), nil)
	}

	if err := s.validateCanvasConfig(params.Canvas); err != nil {
		return nil, NewStreamError(ErrCodeInvalidParams, err.Error(), nil)
	}

	qualityParams := buildQualityParams(params.Bitrate)
	ffmpegOptions := buildFFmpegOptions(params.Options)

	streamConfigTOML := StreamSpec{
		ID:   streamID,
		Name: streamID,
		FFmpeg: FFmpegConfig{
			Codec:         params.Codec,
			QualityParams: qualityParams,
			Options:       ffmpegOptions,
		},
		Canvas:    params.Canvas,
		CreatedAt: time.Now(),
	}

	if err := s.InitializeStream(streamConfigTOML); err != nil {
		return nil, NewStreamError(ErrCodeMonitoringError,
			"failed to initialize stream", err)
	}

	s.streamsMutex.Lock()
	if stream, found := s.streams[streamID]; found {
		stream.InputsEnabled = make(map[string]bool, len(params.Canvas.SourceStreams))
		for _, srcID := range params.Canvas.SourceStreams {
			srcStream, ok := s.streams[srcID]
			stream.InputsEnabled[srcID] = ok && srcStream.Enabled
		}
	}
	s.streamsMutex.Unlock()

	if s.store != nil {
		if err := s.store.AddStream(streamConfigTOML); err != nil {
			s.logger.Warn("Failed to save canvas stream to TOML config", "stream_id", streamID, "error", err)
		} else {
			s.logger.Info("Saved canvas stream to persistent TOML config", "stream_id", streamID)

			// Auto-disable newly-captured source streams before starting the
			// canvas — the source's standalone encoder gets torn down so only
			// the canvas's encoder publishes (producer stays up via refcount).
			s.autoDisableNewSources(context.Background(), params.Canvas.SourceStreams)

			if s.processManager != nil {
				if err := s.processManager.Start(streamID); err != nil {
					s.logger.Warn("Failed to start canvas stream process", "stream_id", streamID, "error", err)
				}
			}
		}
	}

	stream, exists := s.getStreamSafe(streamID)
	if !exists {
		return nil, NewStreamError(ErrCodeStreamNotFound,
			fmt.Sprintf("stream %s was created but not found in memory", streamID), nil)
	}

	return copyStream(stream), nil
}

// UpdateStream updates an existing stream with new parameters.
func (s *service) UpdateStream(ctx context.Context, streamID string, params StreamUpdateParams) (*Stream, error) {
	// Check if stream exists in config
	streamConfig, exists := s.store.GetStream(streamID)
	if !exists {
		return nil, NewStreamError(ErrCodeStreamNotFound, fmt.Sprintf("stream %s not found", streamID), nil)
	}

	// Capture the prior canvas source list so we can detect newly-added
	// sources after the patch and auto-disable them.
	var prevSources []string
	if streamConfig.Canvas != nil {
		prevSources = append(prevSources, streamConfig.Canvas.SourceStreams...)
	}

	// Get in-memory stream for runtime state
	stream, streamExists := s.getStreamSafe(streamID)
	if !streamExists {
		return nil, NewStreamError(ErrCodeStreamNotFound,
			fmt.Sprintf("stream %s not found in memory", streamID), nil)
	}

	// Track if enabled state changed for event emission
	oldEnabled := stream.Enabled
	enabledChanged := false

	streamConfig.FFmpeg.Codec = params.Codec
	streamConfig.FFmpeg.InputFormat = params.InputFormat
	streamConfig.FFmpeg.Resolution = params.Resolution
	streamConfig.FFmpeg.FPS = params.FPS
	streamConfig.FFmpeg.AudioDevice = params.AudioDevice
	streamConfig.FFmpeg.Options = params.Options
	streamConfig.FFmpeg.QualityParams = params.QualityParams
	streamConfig.FFmpeg.Rotation = params.Rotation
	streamConfig.CustomFFmpegCommand = params.CustomFFmpegCommand
	streamConfig.TestMode = params.TestMode
	if params.Canvas != nil {
		if err := s.validateCanvasConfig(params.Canvas); err != nil {
			return nil, NewStreamError(ErrCodeInvalidParams, err.Error(), nil)
		}
	}
	streamConfig.Canvas = params.Canvas
	streamConfig.Perspective = params.Perspective
	streamConfig.Vision = params.Vision

	// Update timestamp
	streamConfig.UpdatedAt = time.Now()

	// Save config to store
	if err := s.store.UpdateStream(streamID, streamConfig); err != nil {
		return nil, fmt.Errorf("failed to save updated stream: %w", err)
	}

	// Update runtime state in-memory
	s.streamsMutex.Lock()
	stream.StartTime = time.Now() // Reset StartTime (stream is effectively restarted)
	if params.Enabled != nil {
		stream.Enabled = *params.Enabled
		if oldEnabled != *params.Enabled {
			enabledChanged = true
		}
	}
	if streamConfig.Canvas != nil {
		s.syncCanvasInputsEnabledLocked(stream, streamConfig.Canvas.SourceStreams)
	}
	s.streamsMutex.Unlock()

	// Auto-disable any sources just added to this canvas (no-op if
	// the stream isn't a canvas or if the source list didn't grow).
	if streamConfig.Canvas != nil {
		added := diffAddedSources(prevSources, streamConfig.Canvas.SourceStreams)
		if len(added) > 0 {
			s.autoDisableNewSources(ctx, added)
		}
	}

	// Unified model: every stream restarts the same way. If a non-canvas
	// source is owned by a canvas, restart the owning canvas instead so
	// the composer picks up the new spec.
	if s.processManager != nil {
		target := streamID
		if streamConfig.Canvas == nil {
			if ownerID := s.processManager.OwnedBy(streamID); ownerID != "" {
				target = ownerID
			}
		}
		if err := s.processManager.Restart(target); err != nil {
			s.logger.Warn("Failed to restart stream process",
				"stream_id", streamID, "target", target, "error", err)
		}
	}

	if enabledChanged && s.eventBus != nil {
		s.eventBus.Publish(events.StreamStateChangedEvent{
			StreamID:  streamID,
			Enabled:   stream.Enabled,
			Timestamp: time.Now().Format(time.RFC3339),
		})
	}

	s.logger.Info("Stream updated successfully", "stream_id", streamID)

	out := copyStream(stream)
	return out, nil
}

// UpdatePartial atomically applies patch to a stream's spec. Holds streamsMutex across load+save.
func (s *service) UpdatePartial(ctx context.Context, streamID string, patch func(*StreamSpec) error) (*Stream, error) {
	if patch == nil {
		return nil, NewStreamError(ErrCodeInvalidParams, "patch function is required", nil)
	}

	s.streamsMutex.Lock()
	streamConfig, exists := s.store.GetStream(streamID)
	if !exists {
		s.streamsMutex.Unlock()
		return nil, NewStreamError(ErrCodeStreamNotFound, fmt.Sprintf("stream %s not found", streamID), nil)
	}
	preimage := streamConfig
	var prevSources []string
	if streamConfig.Canvas != nil {
		prevSources = append(prevSources, streamConfig.Canvas.SourceStreams...)
	}

	stream, streamExists := s.streams[streamID]
	if !streamExists {
		s.streamsMutex.Unlock()
		return nil, NewStreamError(ErrCodeStreamNotFound,
			fmt.Sprintf("stream %s not found in memory", streamID), nil)
	}

	if err := patch(&streamConfig); err != nil {
		s.streamsMutex.Unlock()
		return nil, NewStreamError(ErrCodeInvalidParams, err.Error(), nil)
	}

	if streamConfig.Canvas != nil {
		if err := s.validateCanvasConfig(streamConfig.Canvas); err != nil {
			s.streamsMutex.Unlock()
			return nil, NewStreamError(ErrCodeInvalidParams, err.Error(), nil)
		}
	}

	streamConfig.UpdatedAt = time.Now()
	if err := s.store.UpdateStream(streamID, streamConfig); err != nil {
		s.streamsMutex.Unlock()
		return nil, fmt.Errorf("failed to save updated stream: %w", err)
	}

	if streamConfig.Canvas != nil {
		s.syncCanvasInputsEnabledLocked(stream, streamConfig.Canvas.SourceStreams)
		stream.Enabled = streamConfig.Canvas.IsEngaged()
	}

	stream.StartTime = time.Now()
	streamCopy := copyStream(stream)
	s.streamsMutex.Unlock()

	// Auto-disable any sources just added to this canvas.
	if streamConfig.Canvas != nil {
		added := diffAddedSources(prevSources, streamConfig.Canvas.SourceStreams)
		if len(added) > 0 {
			s.autoDisableNewSources(ctx, added)
		}
	}

	// Live-push perspective to any running composer that has this stream
	// as one of its sources. delivered=true means set_effects crossed the
	// wire; the canvas restart below can be skipped. delivered=false
	// (composer not connected, dispatch failed, or no control server)
	// means we must fall through to a restart so the post-spawn
	// orchestrator delivers the new perspective.
	livePerspectivePushed := false
	if s.processManager != nil && streamConfig.Canvas == nil {
		if ownerID := s.processManager.CanvasOwner(streamID); ownerID != "" {
			delivered, err := s.processManager.PushComposerPerspective(ownerID, streamID, streamConfig.Perspective)
			if err != nil {
				s.logger.Warn("live perspective push failed; restart will deliver via post-spawn orchestrator",
					"canvas_id", ownerID, "source_id", streamID, "error", err)
			}
			livePerspectivePushed = delivered
		}
	}

	if s.processManager != nil {
		switch {
		case streamConfig.Canvas != nil && !streamConfig.Canvas.IsEngaged():
			if err := s.processManager.Stop(streamID); err != nil {
				s.logger.Warn("Failed to stop dormant canvas", "stream_id", streamID, "error", err)
			}
		case streamConfig.Canvas != nil:
			if err := s.processManager.Restart(streamID); err != nil {
				s.logger.Warn("Failed to restart canvas process", "stream_id", streamID, "error", err)
			}
		default:
			canvasID := s.processManager.CanvasOwner(streamID)
			if canvasID != "" && livePerspectivePushed && onlyPerspectiveChanged(preimage, streamConfig) {
				// Live IPC push delivered the new perspective without
				// restart — the canvas stays hot. Skip Restart to
				// preserve the no-restart contract for perspective edits.
				s.logger.Info("perspective updated via IPC; canvas not restarted",
					"canvas_id", canvasID, "source_id", streamID)
			} else if ownerID := s.processManager.OwnedBy(streamID); ownerID != "" {
				if err := s.processManager.Restart(ownerID); err != nil {
					s.logger.Warn("Failed to restart owning canvas after source update",
						"stream_id", streamID, "canvas_id", ownerID, "error", err)
				}
			} else if err := s.processManager.Restart(streamID); err != nil {
				s.logger.Warn("Failed to restart stream process", "stream_id", streamID, "error", err)
			}
		}
	}

	s.logger.Info("Stream updated successfully", "stream_id", streamID)
	return streamCopy, nil
}

// SetEnabled toggles the runtime enabled flag and emits an event on change.
func (s *service) SetEnabled(_ context.Context, streamID string, enabled bool) (bool, error) {
	s.streamsMutex.Lock()
	stream, ok := s.streams[streamID]
	if !ok {
		s.streamsMutex.Unlock()
		return false, NewStreamError(ErrCodeStreamNotFound,
			fmt.Sprintf("stream %s not found", streamID), nil)
	}
	changed := stream.Enabled != enabled
	stream.Enabled = enabled
	s.streamsMutex.Unlock()

	if changed && s.processManager != nil {
		if enabled {
			if err := s.processManager.Start(streamID); err != nil {
				s.logger.Warn("SetEnabled: start failed", "stream_id", streamID, "error", err)
			}
		} else {
			if err := s.processManager.Stop(streamID); err != nil {
				s.logger.Warn("SetEnabled: stop failed", "stream_id", streamID, "error", err)
			}
		}
	}

	if changed && s.eventBus != nil {
		s.eventBus.Publish(events.StreamStateChangedEvent{
			StreamID:  streamID,
			Enabled:   enabled,
			Timestamp: time.Now().Format(time.RFC3339),
		})
	}
	return enabled, nil
}

// DeleteStream removes a stream.
func (s *service) DeleteStream(_ context.Context, streamID string) error {
	// Check if stream exists
	_, exists := s.getStreamSafe(streamID)
	if !exists {
		return NewStreamError(ErrCodeStreamNotFound,
			fmt.Sprintf("stream %s not found", streamID), nil)
	}

	// Stop FFmpeg process first via process manager
	if s.processManager != nil {
		if err := s.processManager.Stop(streamID); err != nil {
			s.logger.Warn("Failed to stop stream process", "stream_id", streamID, "error", err)
		}
	}

	// Remove from store.Store
	if err := s.store.RemoveStream(streamID); err != nil {
		return NewStreamError(ErrCodeConfigError,
			"failed to delete stream from configuration", err)
	}

	// Get stream reference before removing from memory
	stream, _ := s.getStreamSafe(streamID)

	// Remove from memory
	s.streamsMutex.Lock()
	delete(s.streams, streamID)
	s.streamsMutex.Unlock()

	// Stop collector (will also clean up metrics)
	if stream != nil && stream.Collector != nil {
		if err := stream.Collector.Stop(); err != nil {
			s.logger.Warn("Failed to stop stream collector", "stream_id", streamID, "error", err)
		}
	}

	s.logger.Info("Stream deleted successfully", "stream_id", streamID)
	return nil
}

// RestartStream restarts a stream's FFmpeg process and updates StartTime.
func (s *service) RestartStream(_ context.Context, streamID string) error {
	stream, exists := s.getStreamSafe(streamID)
	if !exists {
		return NewStreamError(ErrCodeStreamNotFound,
			fmt.Sprintf("stream %s not found", streamID), nil)
	}

	// Update StartTime in memory
	s.streamsMutex.Lock()
	stream.StartTime = time.Now()
	s.streamsMutex.Unlock()

	// Restart FFmpeg process
	if s.processManager != nil {
		if err := s.processManager.Restart(streamID); err != nil {
			return NewStreamError(ErrCodeProcessError,
				"failed to restart stream process", err)
		}
	}

	s.logger.Info("Stream restarted successfully", "stream_id", streamID)
	return nil
}

// GetStream retrieves a specific stream.
func (s *service) GetStream(_ context.Context, streamID string) (*Stream, error) {
	stream, exists := s.getStreamSafe(streamID)
	if !exists {
		return nil, NewStreamError(ErrCodeStreamNotFound,
			fmt.Sprintf("stream %s not found", streamID), nil)
	}

	out := copyStream(stream)
	return out, nil
}

// GetStreamSpec retrieves the detailed specification of a stream for editing.
func (s *service) GetStreamSpec(_ context.Context, streamID string) (*StreamSpec, error) {
	// Get config from store.Store
	streamConfig, exists := s.store.GetStream(streamID)
	if !exists {
		return nil, NewStreamError(ErrCodeStreamNotFound,
			fmt.Sprintf("stream %s not found", streamID), nil)
	}

	// Return a copy to avoid external mutation
	configCopy := streamConfig
	return &configCopy, nil
}

// ListStreams returns all active streams. Canvases sort before non-canvases so
// they always render first in client UIs; within each group, by ID.
func (s *service) ListStreams(_ context.Context) ([]Stream, error) {
	s.streamsMutex.RLock()
	allSpecs := s.store.GetAllStreams()
	streams := make([]Stream, 0, len(s.streams))
	for _, stream := range s.streams {
		streamCopy := *stream
		streams = append(streams, streamCopy)
	}
	s.streamsMutex.RUnlock()

	sort.Slice(streams, func(i, j int) bool {
		iCanvas := allSpecs[streams[i].ID].Canvas != nil
		jCanvas := allSpecs[streams[j].ID].Canvas != nil
		if iCanvas != jCanvas {
			return iCanvas
		}
		return streams[i].ID < streams[j].ID
	})

	return streams, nil
}

// ListStreamsWithSpecs returns each stream paired with its spec from one snapshot.
func (s *service) ListStreamsWithSpecs(_ context.Context) ([]StreamWithSpec, error) {
	s.streamsMutex.RLock()
	allSpecs := s.store.GetAllStreams()
	out := make([]StreamWithSpec, 0, len(s.streams))
	for id, stream := range s.streams {
		streamCopy := *stream
		spec := allSpecs[id]
		out = append(out, StreamWithSpec{Stream: streamCopy, Spec: spec})
	}
	s.streamsMutex.RUnlock()

	sort.Slice(out, func(i, j int) bool {
		iCanvas := out[i].Spec.Canvas != nil
		jCanvas := out[j].Spec.Canvas != nil
		if iCanvas != jCanvas {
			return iCanvas
		}
		return out[i].Stream.ID < out[j].Stream.ID
	})
	return out, nil
}

// GetProcessManager returns the process manager for shutdown handling.
func (s *service) GetProcessManager() StreamProcessManager {
	return s.processManager
}

// PipelineEnabled reports the persisted master switch state.
func (s *service) PipelineEnabled() bool {
	return s.store.GetPipeline().Enabled
}

// StartPipeline flips the persisted master switch on and starts every
// configured stream. Returns (true, nil) when state actually changed,
// (false, nil) when the pipeline was already running.
func (s *service) StartPipeline(_ context.Context) (bool, error) {
	wasEnabled := s.store.GetPipeline().Enabled
	if err := s.store.SetPipeline(PipelineConfig{Enabled: true}); err != nil {
		return false, fmt.Errorf("persist pipeline state: %w", err)
	}
	if s.processManager != nil {
		if err := s.processManager.StartAll(); err != nil {
			s.logger.Warn("StartPipeline: some streams failed to start", "error", err)
		}
	}
	if !wasEnabled && s.eventBus != nil {
		s.eventBus.Publish(events.PipelineStateChangedEvent{
			Enabled:   true,
			Timestamp: time.Now().Format(time.RFC3339),
		})
	}
	return !wasEnabled, nil
}

// StopPipeline flips the persisted master switch off and stops every
// supervised process. Returns (true, nil) when state actually changed.
func (s *service) StopPipeline(_ context.Context) (bool, error) {
	wasEnabled := s.store.GetPipeline().Enabled
	if err := s.store.SetPipeline(PipelineConfig{Enabled: false}); err != nil {
		return false, fmt.Errorf("persist pipeline state: %w", err)
	}
	if s.processManager != nil {
		s.processManager.StopAll()
	}
	if wasEnabled && s.eventBus != nil {
		s.eventBus.Publish(events.PipelineStateChangedEvent{
			Enabled:   false,
			Timestamp: time.Now().Format(time.RFC3339),
		})
	}
	return wasEnabled, nil
}

// ValidationProvider returns the shared validation accessor backed by the service's store.
func (s *service) ValidationProvider() types.ValidationProvider {
	return s.validationProvider
}

// GetFFmpegCommand returns (command, isCustom, err) for a stream. Builds
// a transient pipeline.EncoderStage from the stored spec and asks it for
// the same argv it would generate at Pool.Start time.
// CustomFFmpegCommand short-circuits to verbatim shell.
// The encoderOverride parameter is accepted for API back-compat but is a
// no-op — the encoder is picked by the Pipeline's backend probe, not
// per-call.
func (s *service) GetFFmpegCommand(_ context.Context, streamID string, _ string) (string, bool, error) {
	streamConfig, exists := s.store.GetStream(streamID)
	if !exists {
		return "", false, NewStreamError(ErrCodeStreamNotFound,
			fmt.Sprintf("stream %s not found", streamID), nil)
	}
	if streamConfig.CustomFFmpegCommand != "" {
		return streamConfig.CustomFFmpegCommand, true, nil
	}
	cmd, err := buildEncoderPreviewCommand(streamConfig, s.rtspHost, s.deviceResolver, s.validationProvider)
	if err != nil {
		return "", false, err
	}
	return cmd, false, nil
}
