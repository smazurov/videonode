package streams

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/smazurov/videonode/internal/encoders"
	"github.com/smazurov/videonode/internal/events"
	"github.com/smazurov/videonode/internal/logging"
	"github.com/smazurov/videonode/internal/types"
)

// ServiceOptions contains optional configuration for the stream service.
type ServiceOptions struct {
	Store            Store // required
	EncoderSelector  encoders.Selector
	EventBus         *events.Bus
	ProcessManager   StreamProcessManager
	VisionDefaultFPS int                   // default FPS for vision pipes; 0 = no throttle
	Native           *NativePipelineConfig // optional; when binaries are present, single V4L2 streams + canvases route through the native dma-buf pipeline
}

type service struct {
	store              Store
	processor          *processor
	canvasProcessor    *canvasProcessor
	streams            map[string]*Stream
	streamsMutex       sync.RWMutex
	processManager     StreamProcessManager
	encoderSelector    encoders.Selector
	validationProvider types.ValidationProvider
	eventBus           *events.Bus
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

	processor := newProcessor(repo)

	encoderSelector := makeEncoderSelector(logger, opts, repo)
	encoderSelectorFunc := makeEncoderSelectorFunc(encoderSelector, logger)
	deviceResolverFunc := makeDeviceResolver(logger)
	processor.setEncoderSelector(encoderSelectorFunc)
	processor.setDeviceResolver(deviceResolverFunc)

	cp := newCanvasProcessor(repo)
	cp.encoderSelector = encoderSelectorFunc
	cp.deviceResolver = deviceResolverFunc
	cp.defaultVisionFPS = opts.VisionDefaultFPS
	cp.native = opts.Native

	processor.native = opts.Native

	svc := &service{
		store:              repo,
		processor:          processor,
		canvasProcessor:    cp,
		streams:            make(map[string]*Stream),
		encoderSelector:    encoderSelector,
		validationProvider: NewValidationService(repo),
		logger:             logger,
	}

	// Wire up processor's access to runtime state
	processor.setStreamStateGetter(svc.getStreamSafe)
	cp.getStreamState = svc.getStreamSafe

	// Apply options
	svc.eventBus = opts.EventBus

	// Initialize process manager
	if opts.ProcessManager != nil {
		svc.processManager = opts.ProcessManager
	} else {
		svc.processManager = NewStreamProcessManager(&ProcessManagerOptions{
			Store:           repo,
			Processor:       processor,
			CanvasProcessor: cp,
			EventBus:        opts.EventBus,
			Native:          opts.Native,
		})
	}

	// Wire up processor's access to crash state
	processor.setIsCrashed(svc.processManager.IsCrashed)
	cp.isCrashed = svc.processManager.IsCrashed

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
	devicePath := s.processor.deviceResolver(params.DeviceID)
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
func (s *service) UpdateStream(_ context.Context, streamID string, params StreamUpdateParams) (*Stream, error) {
	// Check if stream exists in config
	streamConfig, exists := s.store.GetStream(streamID)
	if !exists {
		return nil, NewStreamError(ErrCodeStreamNotFound, fmt.Sprintf("stream %s not found", streamID), nil)
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

	// Canvas members route through RestartCanvas; the standalone process is dormant.
	if s.processManager != nil {
		switch {
		case streamConfig.Canvas != nil:
			if err := s.processManager.RestartCanvas(streamID); err != nil {
				s.logger.Warn("Failed to restart canvas process", "stream_id", streamID, "error", err)
			}
		default:
			if ownerID := s.processManager.OwnedBy(streamID); ownerID != "" {
				if err := s.processManager.RestartCanvas(ownerID); err != nil {
					s.logger.Warn("Failed to restart owning canvas after source update",
						"stream_id", streamID, "canvas_id", ownerID, "error", err)
				}
			} else if err := s.processManager.Restart(streamID); err != nil {
				s.logger.Warn("Failed to restart stream process", "stream_id", streamID, "error", err)
			}
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
	if s.processManager != nil {
		out.OwnedBy = s.processManager.OwnedBy(streamID)
	}
	return out, nil
}

// UpdatePartial atomically applies patch to a stream's spec. Holds streamsMutex across load+save.
func (s *service) UpdatePartial(_ context.Context, streamID string, patch func(*StreamSpec) error) (*Stream, error) {
	if patch == nil {
		return nil, NewStreamError(ErrCodeInvalidParams, "patch function is required", nil)
	}

	s.streamsMutex.Lock()
	streamConfig, exists := s.store.GetStream(streamID)
	if !exists {
		s.streamsMutex.Unlock()
		return nil, NewStreamError(ErrCodeStreamNotFound, fmt.Sprintf("stream %s not found", streamID), nil)
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

	if s.processManager != nil {
		switch {
		case streamConfig.Canvas != nil && !streamConfig.Canvas.IsEngaged():
			if err := s.processManager.ReleaseCanvas(streamID); err != nil {
				s.logger.Warn("Failed to release dormant canvas", "stream_id", streamID, "error", err)
			}
		case streamConfig.Canvas != nil:
			if err := s.processManager.RestartCanvas(streamID); err != nil {
				s.logger.Warn("Failed to restart canvas process", "stream_id", streamID, "error", err)
			}
		default:
			if ownerID := s.processManager.OwnedBy(streamID); ownerID != "" {
				if err := s.processManager.RestartCanvas(ownerID); err != nil {
					s.logger.Warn("Failed to restart owning canvas after source update",
						"stream_id", streamID, "canvas_id", ownerID, "error", err)
				}
			} else if err := s.processManager.Restart(streamID); err != nil {
				s.logger.Warn("Failed to restart stream process", "stream_id", streamID, "error", err)
			}
		}
		streamCopy.OwnedBy = s.processManager.OwnedBy(streamID)
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

// ReleaseCanvas stops a canvas and resumes its sources as standalone streams.
// The canvas spec is preserved; the runtime Stream remains in s.streams with Enabled=false.
func (s *service) ReleaseCanvas(_ context.Context, streamID string) error {
	spec, exists := s.store.GetStream(streamID)
	if !exists {
		return NewStreamError(ErrCodeStreamNotFound,
			fmt.Sprintf("stream %s not found", streamID), nil)
	}
	if spec.Canvas == nil {
		return NewStreamError(ErrCodeInvalidParams,
			fmt.Sprintf("stream %s is not a canvas", streamID), nil)
	}

	if s.processManager != nil {
		if err := s.processManager.ReleaseCanvas(streamID); err != nil {
			return NewStreamError(ErrCodeProcessError,
				"failed to release canvas", err)
		}
	}

	s.streamsMutex.Lock()
	if stream, ok := s.streams[streamID]; ok {
		stream.Enabled = false
	}
	s.streamsMutex.Unlock()

	dormant := false
	spec.Canvas.Enabled = &dormant
	if err := s.store.UpdateStream(streamID, spec); err != nil {
		s.logger.Warn("failed to persist canvas dormant state", "stream_id", streamID, "error", err)
	}

	if s.eventBus != nil {
		s.eventBus.Publish(events.StreamStateChangedEvent{
			StreamID:  streamID,
			Enabled:   false,
			Timestamp: time.Now().Format(time.RFC3339),
		})
	}

	s.logger.Info("Canvas released", "stream_id", streamID)
	return nil
}

// EngageCanvas starts a dormant canvas, claiming its source streams.
// Idempotent: if the canvas is already running and owns all its sources, returns nil.
func (s *service) EngageCanvas(_ context.Context, streamID string) error {
	spec, exists := s.store.GetStream(streamID)
	if !exists {
		return NewStreamError(ErrCodeStreamNotFound,
			fmt.Sprintf("stream %s not found", streamID), nil)
	}
	if spec.Canvas == nil {
		return NewStreamError(ErrCodeInvalidParams,
			fmt.Sprintf("stream %s is not a canvas", streamID), nil)
	}

	if s.processManager != nil && s.processManager.IsRunning(streamID) {
		allOwned := true
		for _, srcID := range spec.Canvas.SourceStreams {
			if s.processManager.OwnedBy(srcID) != streamID {
				allOwned = false
				break
			}
		}
		if allOwned {
			s.streamsMutex.Lock()
			if stream, ok := s.streams[streamID]; ok {
				stream.Enabled = true
			}
			s.streamsMutex.Unlock()
			s.persistCanvasEngaged(streamID, spec)
			return nil
		}
	}

	if s.processManager != nil {
		if err := s.processManager.Start(streamID); err != nil {
			return NewStreamError(ErrCodeProcessError,
				"failed to engage canvas", err)
		}
	}

	s.streamsMutex.Lock()
	if stream, ok := s.streams[streamID]; ok {
		stream.Enabled = true
		stream.StartTime = time.Now()
	}
	s.streamsMutex.Unlock()

	s.persistCanvasEngaged(streamID, spec)

	if s.eventBus != nil {
		s.eventBus.Publish(events.StreamStateChangedEvent{
			StreamID:  streamID,
			Enabled:   true,
			Timestamp: time.Now().Format(time.RFC3339),
		})
	}

	s.logger.Info("Canvas engaged", "stream_id", streamID)
	return nil
}

// persistCanvasEngaged writes Enabled=true to the canvas spec on disk; logs and
// continues on failure since the runtime state is already correct.
func (s *service) persistCanvasEngaged(streamID string, spec StreamSpec) {
	engaged := true
	spec.Canvas.Enabled = &engaged
	if err := s.store.UpdateStream(streamID, spec); err != nil {
		s.logger.Warn("failed to persist canvas engaged state", "stream_id", streamID, "error", err)
	}
}

// GetStream retrieves a specific stream.
func (s *service) GetStream(_ context.Context, streamID string) (*Stream, error) {
	stream, exists := s.getStreamSafe(streamID)
	if !exists {
		return nil, NewStreamError(ErrCodeStreamNotFound,
			fmt.Sprintf("stream %s not found", streamID), nil)
	}

	out := copyStream(stream)
	if s.processManager != nil {
		out.OwnedBy = s.processManager.OwnedBy(streamID)
	}
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
	for id, stream := range s.streams {
		streamCopy := *stream
		if s.processManager != nil {
			streamCopy.OwnedBy = s.processManager.OwnedBy(id)
		}
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
		if s.processManager != nil {
			streamCopy.OwnedBy = s.processManager.OwnedBy(id)
		}
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

// ValidationProvider returns the shared validation accessor backed by the service's store.
func (s *service) ValidationProvider() types.ValidationProvider {
	return s.validationProvider
}

// GetFFmpegCommand returns (command, isCustom, err) for a stream.
func (s *service) GetFFmpegCommand(_ context.Context, streamID string, encoderOverride string) (string, bool, error) {
	streamConfig, exists := s.store.GetStream(streamID)
	if !exists {
		return "", false, NewStreamError(ErrCodeStreamNotFound,
			fmt.Sprintf("stream %s not found", streamID), nil)
	}

	stream, hasState := s.getStreamSafe(streamID)
	enabled := false
	if hasState {
		enabled = stream.Enabled
	}

	if enabled && streamConfig.CustomFFmpegCommand != "" {
		return streamConfig.CustomFFmpegCommand, true, nil
	}

	var processed *ProcessedStream
	var procErr error
	if streamConfig.Canvas != nil {
		processed, procErr = s.canvasProcessor.processStream(streamID)
	} else {
		processed, procErr = s.processor.processStreamWithEncoder(streamID, encoderOverride)
	}
	if procErr != nil {
		return "", false, procErr
	}

	return processed.FFmpegCommand, false, nil
}
