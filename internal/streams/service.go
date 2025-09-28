package streams

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/smazurov/videonode/internal/devices"
	"github.com/smazurov/videonode/internal/encoders"
	"github.com/smazurov/videonode/internal/ffmpeg"
	"github.com/smazurov/videonode/internal/logging"
	"github.com/smazurov/videonode/internal/mediamtx"
	"github.com/smazurov/videonode/internal/obs"
	"github.com/smazurov/videonode/internal/obs/collectors"
	"github.com/smazurov/videonode/internal/types"
	valManager "github.com/smazurov/videonode/internal/validation"
)

// StreamService defines the interface for stream operations
type StreamService interface {
	CreateStream(ctx context.Context, params StreamCreateParams) (*Stream, error)
	UpdateStream(ctx context.Context, streamID string, params StreamUpdateParams) (*Stream, error)
	DeleteStream(ctx context.Context, streamID string) error
	GetStream(ctx context.Context, streamID string) (*Stream, error)
	GetStreamConfig(ctx context.Context, streamID string) (*StreamConfig, error)
	ListStreams(ctx context.Context) ([]Stream, error)
	GetStreamStatus(ctx context.Context, streamID string) (*StreamStatus, error)
	LoadStreamsFromConfig() error
	GetFFmpegCommand(ctx context.Context, streamID string, encoderOverride string) (string, bool, error)
}

// ServiceOptions contains optional configuration for StreamServiceImpl
type ServiceOptions struct {
	OBSManager      *obs.Manager      // OBS manager for monitoring
	EncoderSelector encoders.Selector // Custom encoder selector
}

// StreamServiceImpl implements the StreamService interface
type StreamServiceImpl struct {
	repository      Repository         // Stream repository for data access
	processor       *Processor         // Stream processor for runtime injection
	streams         map[string]*Stream // In-memory stream cache
	streamsMutex    sync.RWMutex
	mediamtxClient  *mediamtx.Client  // MediaMTX API client
	obsManager      *obs.Manager      // OBS manager for monitoring
	encoderSelector encoders.Selector // Encoder selection strategy
	logger          *slog.Logger      // Module logger
}

// NewStreamService creates a new stream service with options
func NewStreamService(opts *ServiceOptions) StreamService {
	logger := logging.GetLogger("streams")

	// Create repository and processor
	repo := NewTOMLRepository("streams.toml")
	if err := repo.Load(); err != nil {
		logger.Warn("Failed to load existing streams", "error", err)
	}

	processor := NewProcessor(repo)

	// Create validation storage and encoder selector
	var encoderSelector encoders.Selector
	if opts != nil && opts.EncoderSelector != nil {
		encoderSelector = opts.EncoderSelector
	} else {
		// Create default encoder selector with validation manager
		validationStorage := NewValidationStorage(repo)
		vm := valManager.NewManager(validationStorage)
		if err := vm.LoadValidation(); err != nil {
			logger.Error("Failed to load validation data", "error", err)
		}
		encoderSelector = encoders.NewDefaultSelector(vm)
	}

	// Configure processor with encoder selector
	processor.SetEncoderSelector(func(codec string, inputFormat string, qualityParams *types.QualityParams, encoderOverride string) *ffmpeg.Params {
		// Convert codec string to CodecType
		var codecType encoders.CodecType
		if codec == "h265" {
			codecType = encoders.CodecH265
		} else {
			codecType = encoders.CodecH264
		}

		// Select optimal encoder (or use override)
		params, err := encoderSelector.SelectEncoder(codecType, inputFormat, qualityParams, encoderOverride)
		if err != nil {
			logger.Error("Failed to select encoder", "error", err)
			// Return defaults
			defaultParams := &ffmpeg.Params{}
			if encoderOverride != "" {
				defaultParams.Encoder = encoderOverride
			} else if codecType == encoders.CodecH265 {
				defaultParams.Encoder = "libx265"
			} else {
				defaultParams.Encoder = "libx264"
			}
			return defaultParams
		}

		return params
	})

	// Configure processor with device resolver
	processor.SetDeviceResolver(func(deviceID string) string {
		devicePath, err := devices.ResolveDevicePath(deviceID)
		if err != nil {
			logger.Warn("Device resolution failed", "device_id", deviceID, "error", err)
			return ""
		}
		return devicePath
	})

	service := &StreamServiceImpl{
		repository:      repo,
		processor:       processor,
		streams:         make(map[string]*Stream),
		encoderSelector: encoderSelector,
		logger:          logger,
	}

	// Apply options if provided
	if opts != nil {
		service.obsManager = opts.OBSManager
	}

	// Initialize MediaMTX API client
	service.mediamtxClient = mediamtx.NewClient("http://localhost:9997", service.getProcessedStreamsForSync)
	service.mediamtxClient.StartHealthMonitor()

	return service
}

// CreateStream creates a new video stream
func (s *StreamServiceImpl) CreateStream(ctx context.Context, params StreamCreateParams) (*Stream, error) {
	// Validate device ID using processor's device resolver
	devicePath := s.processor.deviceResolver(params.DeviceID)
	if devicePath == "" {
		return nil, NewStreamError(ErrCodeDeviceNotFound,
			fmt.Sprintf("device %s not found or not available", params.DeviceID), nil)
	}

	// Use provided stream ID
	streamID := params.StreamID

	// Check if stream already exists
	s.streamsMutex.RLock()
	_, exists := s.streams[streamID]
	s.streamsMutex.RUnlock()

	if exists {
		return nil, NewStreamError(ErrCodeStreamExists,
			fmt.Sprintf("stream %s already exists", streamID), nil)
	}

	// Build resolution string - only if both width and height are provided
	var resolution string
	if params.Width != nil && params.Height != nil && *params.Width > 0 && *params.Height > 0 {
		resolution = fmt.Sprintf("%dx%d", *params.Width, *params.Height)
	}
	// Leave empty if not provided - let device decide

	// Build framerate string - only if provided
	var fps string
	if params.Framerate != nil && *params.Framerate > 0 {
		fps = fmt.Sprintf("%d", *params.Framerate)
	}
	// Leave empty if not provided - let device decide

	// Build quality params from bitrate (CBR mode for now)
	var qualityParams *types.QualityParams
	if params.Bitrate != nil && *params.Bitrate > 0 {
		qualityParams = &types.QualityParams{
			Mode:          types.RateControlCBR,
			TargetBitrate: params.Bitrate, // Already in Mbps
		}
	}

	// Validate that codec is either h264 or h265
	if params.Codec != "h264" && params.Codec != "h265" {
		return nil, NewStreamError(ErrCodeInvalidParams,
			fmt.Sprintf("invalid codec: %s (must be h264 or h265)", params.Codec), nil)
	}

	// Determine which FFmpeg options to use
	var ffmpegOptions []ffmpeg.OptionType
	if len(params.Options) > 0 {
		// Use user-provided options (convert string to OptionType)
		for _, opt := range params.Options {
			ffmpegOptions = append(ffmpegOptions, ffmpeg.OptionType(opt))
		}
	} else {
		// Use default options
		ffmpegOptions = ffmpeg.GetDefaultOptions()
	}

	// Create stream configuration with FFmpeg section
	// Store only the generic codec, not the specific encoder
	streamConfigTOML := StreamConfig{
		ID:      streamID,
		Name:    streamID,
		Device:  params.DeviceID, // Store stable device ID
		Enabled: true,
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

	// Save to persistent TOML config
	if s.repository != nil {
		if err := s.repository.AddStream(streamConfigTOML); err != nil {
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
	s.streamsMutex.RLock()
	stream, exists := s.streams[streamID]
	s.streamsMutex.RUnlock()

	if !exists {
		return nil, NewStreamError(ErrCodeStreamNotFound,
			fmt.Sprintf("stream %s was created but not found in memory", streamID), nil)
	}

	// Return a copy to avoid external mutation
	streamCopy := *stream
	return &streamCopy, nil
}

// UpdateStream updates an existing stream with new parameters
func (s *StreamServiceImpl) UpdateStream(ctx context.Context, streamID string, params StreamUpdateParams) (*Stream, error) {
	// Check if stream exists
	streamConfig, exists := s.repository.GetStream(streamID)
	if !exists {
		return nil, NewStreamError(ErrCodeStreamNotFound, fmt.Sprintf("stream %s not found", streamID), nil)
	}

	// Update stream configuration with provided parameters
	if params.Codec != nil {
		streamConfig.FFmpeg.Codec = *params.Codec
	}
	if params.InputFormat != nil {
		streamConfig.FFmpeg.InputFormat = *params.InputFormat
	}
	if params.Width != nil && params.Height != nil {
		streamConfig.FFmpeg.Resolution = fmt.Sprintf("%dx%d", *params.Width, *params.Height)
	}
	if params.Framerate != nil {
		streamConfig.FFmpeg.FPS = fmt.Sprintf("%d", *params.Framerate)
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

	// Save to repository
	if err := s.repository.UpdateStream(streamID, streamConfig); err != nil {
		return nil, fmt.Errorf("failed to save updated stream: %w", err)
	}

	// Generate new FFmpeg command and update MediaMTX
	processed, err := s.processor.ProcessStream(streamID)
	if err != nil {
		return nil, fmt.Errorf("failed to process updated stream: %w", err)
	}

	// Update in MediaMTX via API
	if s.mediamtxClient != nil {
		if err := s.mediamtxClient.UpdatePath(streamID, processed.FFmpegCommand); err != nil {
			s.logger.Warn("Failed to update MediaMTX path", "stream_id", streamID, "error", err)
		}
	}

	// Reset StartTime when stream is patched (stream is effectively restarted)
	s.streamsMutex.Lock()
	stream, exists := s.streams[streamID]
	if exists {
		stream.StartTime = time.Now()
	}
	s.streamsMutex.Unlock()

	if !exists {
		return nil, NewStreamError(ErrCodeStreamNotFound,
			fmt.Sprintf("updated stream %s not found in memory", streamID), nil)
	}

	s.logger.Info("Stream updated successfully", "stream_id", streamID)

	// Return a copy to avoid external mutation
	streamCopy := *stream
	return &streamCopy, nil
}

// DeleteStream removes a stream
func (s *StreamServiceImpl) DeleteStream(ctx context.Context, streamID string) error {
	// Check if stream exists
	s.streamsMutex.RLock()
	_, exists := s.streams[streamID]
	s.streamsMutex.RUnlock()

	if !exists {
		return NewStreamError(ErrCodeStreamNotFound,
			fmt.Sprintf("stream %s not found", streamID), nil)
	}

	// Remove from repository
	if err := s.repository.RemoveStream(streamID); err != nil {
		return NewStreamError(ErrCodeConfigError,
			"failed to delete stream from configuration", err)
	}

	// Remove from memory
	s.streamsMutex.Lock()
	delete(s.streams, streamID)
	s.streamsMutex.Unlock()

	// Remove from MediaMTX via API (ignore errors, will sync on reconnect)
	if s.mediamtxClient != nil {
		_ = s.mediamtxClient.DeletePath(streamID)
	}

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
func (s *StreamServiceImpl) GetStream(ctx context.Context, streamID string) (*Stream, error) {
	s.streamsMutex.RLock()
	stream, exists := s.streams[streamID]
	s.streamsMutex.RUnlock()

	if !exists {
		return nil, NewStreamError(ErrCodeStreamNotFound,
			fmt.Sprintf("stream %s not found", streamID), nil)
	}

	// Return a copy to avoid external mutation
	streamCopy := *stream
	return &streamCopy, nil
}

// GetStreamConfig retrieves the detailed configuration of a stream for editing
func (s *StreamServiceImpl) GetStreamConfig(ctx context.Context, streamID string) (*StreamConfig, error) {
	// Get config from repository
	streamConfig, exists := s.repository.GetStream(streamID)
	if !exists {
		return nil, NewStreamError(ErrCodeStreamNotFound,
			fmt.Sprintf("stream %s not found", streamID), nil)
	}

	// Return a copy to avoid external mutation
	configCopy := streamConfig
	return &configCopy, nil
}

// ListStreams returns all active streams
func (s *StreamServiceImpl) ListStreams(ctx context.Context) ([]Stream, error) {
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

// GetStreamStatus returns the runtime status of a stream
func (s *StreamServiceImpl) GetStreamStatus(ctx context.Context, streamID string) (*StreamStatus, error) {
	s.streamsMutex.RLock()
	stream, exists := s.streams[streamID]
	s.streamsMutex.RUnlock()

	if !exists {
		return nil, NewStreamError(ErrCodeStreamNotFound,
			fmt.Sprintf("stream %s not found", streamID), nil)
	}

	status := &StreamStatus{
		StreamID:  stream.ID,
		StartTime: stream.StartTime,
	}

	return status, nil
}

// LoadStreamsFromConfig loads existing streams from TOML config into memory
func (s *StreamServiceImpl) LoadStreamsFromConfig() error {
	if s.repository == nil {
		return fmt.Errorf("repository not initialized")
	}

	// Load the configuration from file
	if err := s.repository.Load(); err != nil {
		return fmt.Errorf("failed to load streams configuration: %w", err)
	}

	streams := s.repository.GetAllStreams()
	// No lock needed here - InitializeStream handles its own locking

	for _, streamConfig := range streams {
		// Only load enabled streams
		if !streamConfig.Enabled {
			continue
		}

		// Initialize the stream with all integrations
		if err := s.InitializeStream(streamConfig); err != nil {
			s.logger.Warn("Failed to initialize stream", "stream_id", streamConfig.ID, "error", err)
			continue
		}
	}

	// Need to read the count safely
	s.streamsMutex.RLock()
	streamCount := len(s.streams)
	s.streamsMutex.RUnlock()

	s.logger.Info("Loaded streams from configuration", "count", streamCount)

	// Sync all streams to MediaMTX via API
	if s.mediamtxClient != nil {
		if err := s.mediamtxClient.SyncAll(); err != nil {
			s.logger.Warn("Failed to sync MediaMTX at startup", "error", err)
		}
	}

	// Sync streams to MediaMTX
	if err := s.syncToMediaMTX(); err != nil {
		s.logger.Warn("Failed to sync MediaMTX at startup", "error", err)
	}
	return nil
}

// InitializeStream initializes a single stream with all integrations
func (s *StreamServiceImpl) InitializeStream(streamConfig StreamConfig) error {
	// Create stream entity from config
	stream := &Stream{
		ID:        streamConfig.ID,
		DeviceID:  streamConfig.Device,
		Codec:     streamConfig.FFmpeg.Codec, // Use the generic codec from FFmpeg config
		StartTime: time.Now(),                // Track when loaded into memory, not creation time
	}

	// Store the stream in memory - only lock for the write
	s.streamsMutex.Lock()
	s.streams[streamConfig.ID] = stream
	stream.ProgressSocket = GetSocketPath(streamConfig.ID)
	s.streamsMutex.Unlock()

	// Initialize OBS monitoring
	if s.obsManager != nil {
		socketPath := GetSocketPath(streamConfig.ID)
		ffmpegCollector := collectors.NewFFmpegCollector(socketPath, "", streamConfig.ID)
		if err := s.obsManager.AddCollector(ffmpegCollector); err != nil {
			s.logger.Warn("Failed to register OBS collector for stream", "stream_id", streamConfig.ID, "error", err)
		}
	}

	s.logger.Info("Initialized stream", "stream_id", streamConfig.ID, "device", streamConfig.Device, "codec", streamConfig.FFmpeg.Codec)
	return nil
}

// syncToMediaMTX syncs all streams to MediaMTX using the API
func (s *StreamServiceImpl) syncToMediaMTX() error {
	// Process all streams to generate FFmpeg commands
	processedStreams, err := s.processor.ProcessAllStreams()
	if err != nil {
		return fmt.Errorf("failed to process streams: %w", err)
	}

	// Restart each stream to ensure fresh FFmpeg processes connect to progress sockets
	if s.mediamtxClient != nil {
		for _, stream := range processedStreams {
			if stream.FFmpegCommand != "" {
				// Force restart to ensure FFmpeg connects to socket listeners
				_ = s.mediamtxClient.RestartPath(stream.StreamID, stream.FFmpegCommand)
			}
		}
	}

	s.logger.Info("Synchronized streams with MediaMTX API", "count", len(processedStreams))
	return nil
}

// getProcessedStreamsForSync is a callback for the MediaMTX client to get all streams
func (s *StreamServiceImpl) getProcessedStreamsForSync() []*mediamtx.ProcessedStream {
	processedStreams, err := s.processor.ProcessAllStreams()
	if err != nil {
		s.logger.Error("Failed to process streams for sync", "error", err)
		return nil
	}

	// Convert to MediaMTX ProcessedStream format
	result := make([]*mediamtx.ProcessedStream, 0, len(processedStreams))
	for _, stream := range processedStreams {
		if stream.FFmpegCommand != "" {
			result = append(result, &mediamtx.ProcessedStream{
				StreamID:      stream.StreamID,
				FFmpegCommand: stream.FFmpegCommand,
			})
		}
	}
	return result
}

// GetFFmpegCommand retrieves the FFmpeg command for a stream
// Returns the command and a boolean indicating if it's custom
func (s *StreamServiceImpl) GetFFmpegCommand(ctx context.Context, streamID string, encoderOverride string) (string, bool, error) {
	// Check if stream exists
	streamConfig, exists := s.repository.GetStream(streamID)
	if !exists {
		return "", false, NewStreamError(ErrCodeStreamNotFound, fmt.Sprintf("stream %s not found", streamID), nil)
	}

	// If custom command is set, return it
	if streamConfig.CustomFFmpegCommand != "" {
		return mediamtx.WrapCommand(streamConfig.CustomFFmpegCommand, streamID), true, nil
	}

	// Otherwise, process the stream to generate the command
	processed, err := s.processor.ProcessStreamWithEncoder(streamID, encoderOverride)
	if err != nil {
		return "", false, fmt.Errorf("failed to generate FFmpeg command: %w", err)
	}

	return mediamtx.WrapCommand(processed.FFmpegCommand, streamID), false, nil
}
