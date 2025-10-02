package streams

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/smazurov/videonode/internal/encoders"
	"github.com/smazurov/videonode/internal/events"
	"github.com/smazurov/videonode/internal/ffmpeg"
	"github.com/smazurov/videonode/internal/logging"
	"github.com/smazurov/videonode/internal/mediamtx"
	"github.com/smazurov/videonode/internal/obs"
	"github.com/smazurov/videonode/internal/types"
)

// ServiceOptions contains optional configuration for StreamServiceImpl
type ServiceOptions struct {
	Store           Store             // Stream store for persistence (required)
	OBSManager      *obs.Manager      // OBS manager for monitoring
	EncoderSelector encoders.Selector // Custom encoder selector
	EventBus        *events.Bus       // Event bus for broadcasting state changes
}

// service implements the StreamService interface
type service struct {
	store           Store             // Stream store for data access
	processor       *processor        // Stream processor for runtime injection
	streams         map[string]*Stream // In-memory stream cache
	streamsMutex    sync.RWMutex
	mediamtxClient  *mediamtx.Client  // MediaMTX API client
	obsManager      *obs.Manager      // OBS manager for monitoring
	encoderSelector encoders.Selector // Encoder selection strategy
	eventBus        *events.Bus       // Event bus for broadcasting state changes
	logger          *slog.Logger      // Module logger
}

// NewStreamService creates a new stream service with options
func NewStreamService(opts *ServiceOptions) *service {
	logger := logging.GetLogger("streams")

	if opts == nil || opts.Store == nil {
		logger.Error("Store is required in ServiceOptions")
		panic("Store is required in ServiceOptions")
	}

	repo := opts.Store
	if err := repo.Load(); err != nil {
		logger.Warn("Failed to load existing streams", "error", err)
	}

	processor := newProcessor(repo)

	// Configure processor with extracted helpers
	encoderSelector := makeEncoderSelector(logger, opts, repo)
	processor.setEncoderSelector(makeEncoderSelectorFunc(encoderSelector, logger))
	processor.setDeviceResolver(makeDeviceResolver(logger))

	// Create service
	svc := &service{
		store:           repo,
		processor:       processor,
		streams:         make(map[string]*Stream),
		encoderSelector: encoderSelector,
		logger:          logger,
	}

	// Wire up processor's access to runtime state
	processor.setStreamStateGetter(svc.getStreamSafe)

	// Apply options if provided
	if opts != nil {
		svc.obsManager = opts.OBSManager
		svc.eventBus = opts.EventBus
	}

	// Initialize MediaMTX API client
	svc.mediamtxClient = setupMediaMTXClient(svc)

	return svc
}

// CreateStream creates a new video stream
func (s *service) CreateStream(ctx context.Context, params StreamCreateParams) (*Stream, error) {
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

	// Create stream configuration with FFmpeg section
	// Store only the generic codec, not the specific encoder
	streamConfigTOML := StreamSpec{
		ID:      streamID,
		Name:    streamID,
		Device:  params.DeviceID, // Store stable device ID
		FFmpeg: FFmpegConfig{
			Codec:         params.Codec, // Store generic codec (h264/h265), not specific encoder
			InputFormat:   params.InputFormat,
			Resolution:    resolution,
			FPS:           fps,
			Options:       ffmpegOptions,      // Apply user-provided or default options
			QualityParams: qualityParams,      // Store quality params for future use
			AudioDevice:   params.AudioDevice, // Pass through audio device if specified
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
	if stream, exists := s.streams[streamID]; exists {
		stream.Enabled = true
	}
	s.streamsMutex.Unlock()

	// Save to persistent TOML config
	if s.store != nil {
		if err := s.store.AddStream(streamConfigTOML); err != nil {
			s.logger.Warn("Failed to save stream to TOML config", "stream_id", streamID, "error", err)
		} else {
			s.logger.Info("Saved stream to persistent TOML config", "stream_id", streamID)

			// Sync to MediaMTX API
			if err := s.syncToMediaMTX(); err != nil {
				s.logger.Warn("Failed to sync MediaMTX after creation", "error", err)
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

// UpdateStream updates an existing stream with new parameters
func (s *service) UpdateStream(ctx context.Context, streamID string, params StreamUpdateParams) (*Stream, error) {
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

	// Update stream configuration with provided parameters
	if params.Codec != nil {
		streamConfig.FFmpeg.Codec = *params.Codec
	}
	if params.InputFormat != nil {
		streamConfig.FFmpeg.InputFormat = *params.InputFormat
	}
	if params.Width != nil && params.Height != nil {
		streamConfig.FFmpeg.Resolution = formatResolution(params.Width, params.Height)
	}
	if params.Framerate != nil {
		streamConfig.FFmpeg.FPS = formatFPS(params.Framerate)
	}
	if params.AudioDevice != nil {
		streamConfig.FFmpeg.AudioDevice = *params.AudioDevice
	}
	if params.Options != nil {
		// Convert string slice to OptionType slice
		var ffmpegOptions []ffmpeg.OptionType
		for _, opt := range params.Options {
			ffmpegOptions = append(ffmpegOptions, ffmpeg.OptionType(opt))
		}
		streamConfig.FFmpeg.Options = ffmpegOptions
	}
	if params.Bitrate != nil {
		// Update quality params
		if streamConfig.FFmpeg.QualityParams == nil {
			streamConfig.FFmpeg.QualityParams = &types.QualityParams{}
		}
		streamConfig.FFmpeg.QualityParams.TargetBitrate = params.Bitrate
	}
	if params.CustomFFmpegCommand != nil {
		streamConfig.CustomFFmpegCommand = *params.CustomFFmpegCommand
	}
	if params.TestMode != nil {
		streamConfig.TestMode = *params.TestMode
	}

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
	s.streamsMutex.Unlock()

	// Generate new FFmpeg command and update MediaMTX
	processed, err := s.processor.processStream(streamID)
	if err != nil {
		return nil, fmt.Errorf("failed to process updated stream: %w", err)
	}

	// Update in MediaMTX via API
	if err := s.mediamtxClient.UpdatePath(streamID, processed.FFmpegCommand); err != nil {
		s.logger.Warn("Failed to update MediaMTX path", "stream_id", streamID, "error", err)
	}

	// Emit stream state changed event if enabled state was modified
	if enabledChanged && s.eventBus != nil {
		s.eventBus.Publish(events.StreamStateChangedEvent{
			StreamID:  streamID,
			Enabled:   stream.Enabled,
			Timestamp: time.Now().Format(time.RFC3339),
		})
	}

	s.logger.Info("Stream updated successfully", "stream_id", streamID)

	return copyStream(stream), nil
}

// DeleteStream removes a stream
func (s *service) DeleteStream(ctx context.Context, streamID string) error {
	// Check if stream exists
	_, exists := s.getStreamSafe(streamID)
	if !exists {
		return NewStreamError(ErrCodeStreamNotFound,
			fmt.Sprintf("stream %s not found", streamID), nil)
	}

	// Remove from store.Store
	if err := s.store.RemoveStream(streamID); err != nil {
		return NewStreamError(ErrCodeConfigError,
			"failed to delete stream from configuration", err)
	}

	// Remove from memory
	s.streamsMutex.Lock()
	delete(s.streams, streamID)
	s.streamsMutex.Unlock()

	// Remove from MediaMTX via API (ignore errors, will sync on reconnect)
	_ = s.mediamtxClient.DeletePath(streamID)

	// Remove OBS monitoring if configured
	if s.obsManager != nil {
		collectorName := "ffmpeg_" + streamID
		if err := s.obsManager.RemoveCollector(collectorName); err != nil {
			s.logger.Warn("Failed to remove OBS monitoring for stream", "stream_id", streamID, "error", err)
		}
	}

	s.logger.Info("Stream deleted successfully", "stream_id", streamID)
	return nil
}

// GetStream retrieves a specific stream
func (s *service) GetStream(ctx context.Context, streamID string) (*Stream, error) {
	stream, exists := s.getStreamSafe(streamID)
	if !exists {
		return nil, NewStreamError(ErrCodeStreamNotFound,
			fmt.Sprintf("stream %s not found", streamID), nil)
	}

	return copyStream(stream), nil
}

// GetStreamSpec retrieves the detailed specification of a stream for editing
func (s *service) GetStreamSpec(ctx context.Context, streamID string) (*StreamSpec, error) {
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

// ListStreams returns all active streams
func (s *service) ListStreams(ctx context.Context) ([]Stream, error) {
	s.streamsMutex.RLock()
	defer s.streamsMutex.RUnlock()

	streams := make([]Stream, 0, len(s.streams))
	for _, stream := range s.streams {
		// Return copies to avoid external mutation
		streamCopy := *stream
		streams = append(streams, streamCopy)
	}

	// Sort streams by ID for consistent ordering
	sort.Slice(streams, func(i, j int) bool {
		return streams[i].ID < streams[j].ID
	})

	return streams, nil
}
