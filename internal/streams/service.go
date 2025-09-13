package streams

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/smazurov/videonode/internal/devices"
	"github.com/smazurov/videonode/internal/encoders"
	"github.com/smazurov/videonode/internal/ffmpeg"
	"github.com/smazurov/videonode/internal/mediamtx"
	"github.com/smazurov/videonode/internal/types"
	valManager "github.com/smazurov/videonode/internal/validation"
)

// StreamService defines the interface for stream operations
type StreamService interface {
	CreateStream(ctx context.Context, params StreamCreateParams) (*Stream, error)
	DeleteStream(ctx context.Context, streamID string) error
	GetStream(ctx context.Context, streamID string) (*Stream, error)
	ListStreams(ctx context.Context) ([]Stream, error)
	GetStreamStatus(ctx context.Context, streamID string) (*StreamStatus, error)
	LoadStreamsFromConfig() error
	GetFFmpegCommand(ctx context.Context, streamID string, encoderOverride string) (string, bool, error)
	SetCustomFFmpegCommand(ctx context.Context, streamID string, command string) error
	ClearCustomFFmpegCommand(ctx context.Context, streamID string) error
}

// ServiceOptions contains optional configuration for StreamServiceImpl
type ServiceOptions struct {
	OBSIntegration  func(string, string, string) error // Function to add OBS monitoring
	OBSRemoval      func(string) error                 // Function to remove OBS monitoring
	EncoderSelector encoders.Selector                  // Custom encoder selector
}

// StreamServiceImpl implements the StreamService interface
type StreamServiceImpl struct {
	repository      Repository         // Stream repository for data access
	processor       *Processor         // Stream processor for runtime injection
	streams         map[string]*Stream // In-memory stream cache
	streamsMutex    sync.RWMutex
	mediamtxClient  *mediamtx.Client                   // MediaMTX API client
	obsIntegration  func(string, string, string) error // Function to add OBS monitoring
	obsRemoval      func(string) error                 // Function to remove OBS monitoring
	encoderSelector encoders.Selector                  // Encoder selection strategy
}

// NewStreamService creates a new stream service with options
func NewStreamService(opts *ServiceOptions) StreamService {
	// Create repository and processor
	repo := NewTOMLRepository("streams.toml")
	if err := repo.Load(); err != nil {
		log.Printf("Warning: Failed to load existing streams: %v", err)
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
			log.Printf("Failed to load validation data: %v", err)
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
			log.Printf("Failed to select encoder: %v", err)
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
			log.Printf("Device resolution failed for %s: %v", deviceID, err)
			return ""
		}
		return devicePath
	})

	// Configure processor with socket creator
	processor.SetSocketCreator(func(streamID string) string {
		// Use monotonic socket path - deterministic based on stream ID
		socketPath := fmt.Sprintf("/tmp/ffmpeg-progress-%s.sock", streamID)
		return socketPath
	})

	service := &StreamServiceImpl{
		repository:      repo,
		processor:       processor,
		streams:         make(map[string]*Stream),
		encoderSelector: encoderSelector,
	}

	// Apply options if provided
	if opts != nil {
		service.obsIntegration = opts.OBSIntegration
		service.obsRemoval = opts.OBSRemoval
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
			log.Printf("Warning: Failed to save stream to TOML config: %v", err)
		} else {
			log.Printf("Saved stream %s to persistent TOML config", streamID)

			// Sync to MediaMTX API
			socketPaths, err := s.syncToMediaMTX()
			if err != nil {
				log.Printf("Warning: Failed to sync MediaMTX after creation: %v", err)
			} else {
				// Update OBS collectors with new socket paths
				s.updateSocketPaths(socketPaths)
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
	if s.obsRemoval != nil {
		if err := s.obsRemoval(streamID); err != nil {
			log.Printf("Warning: Failed to remove OBS monitoring for stream %s: %v", streamID, err)
		}
	}

	log.Printf("Stream %s deleted successfully", streamID)
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
		Uptime:    time.Since(stream.StartTime),
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
			log.Printf("Warning: Failed to initialize stream %s: %v", streamConfig.ID, err)
			continue
		}
	}

	// Need to read the count safely
	s.streamsMutex.RLock()
	streamCount := len(s.streams)
	s.streamsMutex.RUnlock()

	log.Printf("Loaded %d streams from configuration", streamCount)

	// Sync all streams to MediaMTX via API
	if s.mediamtxClient != nil {
		if err := s.mediamtxClient.SyncAll(); err != nil {
			log.Printf("Warning: Failed to sync MediaMTX at startup: %v", err)
		}
	}

	// Generate socket paths for OBS monitoring
	socketPaths, err := s.syncToMediaMTX()
	if err != nil {
		log.Printf("Warning: Failed to generate socket paths: %v", err)
	} else {
		s.updateSocketPaths(socketPaths)
	}
	return nil
}

// InitializeStream initializes a single stream with all integrations
func (s *StreamServiceImpl) InitializeStream(streamConfig StreamConfig) error {
	// Extract display bitrate from quality params if available
	displayBitrate := "2M" // Default
	if streamConfig.FFmpeg.QualityParams != nil && streamConfig.FFmpeg.QualityParams.TargetBitrate != nil {
		displayBitrate = fmt.Sprintf("%.1fM", *streamConfig.FFmpeg.QualityParams.TargetBitrate)
	}

	// Create stream entity from config
	stream := &Stream{
		ID:        streamConfig.ID,
		DeviceID:  streamConfig.Device,
		Codec:     streamConfig.FFmpeg.Codec, // Use the generic codec from FFmpeg config
		Bitrate:   displayBitrate,            // Display bitrate from quality params
		StartTime: streamConfig.CreatedAt,
		WebRTCURL: fmt.Sprintf(":8889/%s", streamConfig.ID),
		RTSPURL:   fmt.Sprintf(":8554/%s", streamConfig.ID),
	}

	// Store the stream in memory - only lock for the write
	s.streamsMutex.Lock()
	s.streams[streamConfig.ID] = stream
	s.streamsMutex.Unlock()

	// OBS monitoring will be initialized when socket paths are generated during MediaMTX sync
	log.Printf("Initialized stream %s (device: %s, codec: %s)", streamConfig.ID, streamConfig.Device, streamConfig.FFmpeg.Codec)
	return nil
}

// syncToMediaMTX syncs all streams to MediaMTX using the API
func (s *StreamServiceImpl) syncToMediaMTX() (map[string]string, error) {
	// Track socket paths that get generated
	socketPaths := make(map[string]string)

	// Configure processor's socket creator to track paths
	oldSocketCreator := s.processor.socketCreator
	s.processor.SetSocketCreator(func(streamID string) string {
		socketPath := oldSocketCreator(streamID)
		socketPaths[streamID] = socketPath
		return socketPath
	})

	// Process all streams to generate FFmpeg commands
	processedStreams, err := s.processor.ProcessAllStreams()
	if err != nil {
		return nil, fmt.Errorf("failed to process streams: %w", err)
	}

	// Add each stream to MediaMTX via API
	if s.mediamtxClient != nil {
		for _, stream := range processedStreams {
			if stream.FFmpegCommand != "" {
				// Try to add/update the path, ignore errors (will sync on reconnect)
				_ = s.mediamtxClient.AddPath(stream.StreamID, stream.FFmpegCommand)
			}
		}
	}

	log.Printf("Synchronized %d streams with MediaMTX API", len(processedStreams))
	return socketPaths, nil
}

// getProcessedStreamsForSync is a callback for the MediaMTX client to get all streams
func (s *StreamServiceImpl) getProcessedStreamsForSync() []*mediamtx.ProcessedStream {
	processedStreams, err := s.processor.ProcessAllStreams()
	if err != nil {
		log.Printf("Failed to process streams for sync: %v", err)
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

// updateSocketPaths updates the runtime socket paths for streams and their OBS collectors
func (s *StreamServiceImpl) updateSocketPaths(socketPaths map[string]string) {
	if socketPaths == nil {
		return
	}

	s.streamsMutex.Lock()
	defer s.streamsMutex.Unlock()

	for streamID, socketPath := range socketPaths {
		stream, exists := s.streams[streamID]
		if !exists {
			continue
		}

		// Update the runtime socket path
		stream.ProgressSocket = socketPath

		// Update OBS monitoring with new socket path
		if s.obsIntegration != nil && socketPath != "" {
			// First remove the old collector if it exists
			if s.obsRemoval != nil {
				if err := s.obsRemoval(streamID); err != nil {
					log.Printf("Warning: Failed to cleanup old OBS collector for stream %s: %v", streamID, err)
				}
			}

			// Add new collector with fresh socket path
			logPath := "" // No log path for now, socket monitoring only
			if err := s.obsIntegration(streamID, socketPath, logPath); err != nil {
				log.Printf("Warning: Failed to register OBS collector for stream %s: %v", streamID, err)
			}
		}
	}
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
		return streamConfig.CustomFFmpegCommand, true, nil
	}

	// Otherwise, process the stream to generate the command
	processed, err := s.processor.ProcessStreamWithEncoder(streamID, encoderOverride)
	if err != nil {
		return "", false, fmt.Errorf("failed to generate FFmpeg command: %w", err)
	}

	return processed.FFmpegCommand, false, nil
}

// SetCustomFFmpegCommand sets a custom FFmpeg command for a stream
func (s *StreamServiceImpl) SetCustomFFmpegCommand(ctx context.Context, streamID string, command string) error {
	// Check if stream exists
	streamConfig, exists := s.repository.GetStream(streamID)
	if !exists {
		return NewStreamError(ErrCodeStreamNotFound, fmt.Sprintf("stream %s not found", streamID), nil)
	}

	// Basic validation - command should start with "ffmpeg"
	if !strings.HasPrefix(strings.TrimSpace(command), "ffmpeg") {
		return NewStreamError(ErrCodeInvalidParams, "custom command must start with 'ffmpeg'", nil)
	}

	// Update the stream configuration
	streamConfig.CustomFFmpegCommand = command
	streamConfig.UpdatedAt = time.Now()

	// Save to repository
	if err := s.repository.UpdateStream(streamID, streamConfig); err != nil {
		return fmt.Errorf("failed to save custom FFmpeg command: %w", err)
	}

	// Generate new FFmpeg command and update MediaMTX
	processed, err := s.processor.ProcessStream(streamID)
	if err != nil {
		// Try to rollback
		streamConfig.CustomFFmpegCommand = ""
		s.repository.UpdateStream(streamID, streamConfig)
		return fmt.Errorf("failed to process stream: %w", err)
	}

	// Update in MediaMTX via API
	if s.mediamtxClient != nil {
		_ = s.mediamtxClient.UpdatePath(streamID, processed.FFmpegCommand)
	}

	// Update socket paths for OBS monitoring
	if processed.SocketPath != "" {
		socketPaths := map[string]string{streamID: processed.SocketPath}
		s.updateSocketPaths(socketPaths)
	}

	log.Printf("Set custom FFmpeg command for stream %s", streamID)
	return nil
}

// ClearCustomFFmpegCommand removes the custom FFmpeg command for a stream
func (s *StreamServiceImpl) ClearCustomFFmpegCommand(ctx context.Context, streamID string) error {
	// Check if stream exists
	streamConfig, exists := s.repository.GetStream(streamID)
	if !exists {
		return NewStreamError(ErrCodeStreamNotFound, fmt.Sprintf("stream %s not found", streamID), nil)
	}

	// Clear the custom command
	streamConfig.CustomFFmpegCommand = ""
	streamConfig.UpdatedAt = time.Now()

	// Save to repository
	if err := s.repository.UpdateStream(streamID, streamConfig); err != nil {
		return fmt.Errorf("failed to clear custom FFmpeg command: %w", err)
	}

	// Generate new FFmpeg command and update MediaMTX
	processed, err := s.processor.ProcessStream(streamID)
	if err != nil {
		return fmt.Errorf("failed to process stream: %w", err)
	}

	// Update in MediaMTX via API
	if s.mediamtxClient != nil {
		_ = s.mediamtxClient.UpdatePath(streamID, processed.FFmpegCommand)
	}

	// Update socket paths for OBS monitoring
	if processed.SocketPath != "" {
		socketPaths := map[string]string{streamID: processed.SocketPath}
		s.updateSocketPaths(socketPaths)
	}

	log.Printf("Cleared custom FFmpeg command for stream %s", streamID)
	return nil
}
