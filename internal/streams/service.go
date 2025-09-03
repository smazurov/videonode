package streams

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	streamconfig "github.com/smazurov/videonode/internal/config"
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
}

// StreamServiceImpl implements the StreamService interface
type StreamServiceImpl struct {
	streamManager   *streamconfig.StreamManager
	streams         map[string]*Stream
	streamsMutex    sync.RWMutex
	mediamtxConfig  string
	obsIntegration  func(string, string, string) error // Function to add OBS monitoring
	obsRemoval      func(string) error                 // Function to remove OBS monitoring
	encoderSelector encoders.Selector                  // Encoder selection strategy
}

// NewStreamService creates a new stream service
func NewStreamService(streamManager *streamconfig.StreamManager, mediamtxConfigPath string) StreamService {
	// Create default encoder selector using stream manager for backward compatibility
	storage := valManager.NewStreamStorage(streamManager)
	vm := valManager.NewManager(storage)
	if err := vm.LoadValidation(); err != nil {
		log.Printf("Failed to load validation data: %v", err)
	}
	selector := encoders.NewDefaultSelector(vm)

	return &StreamServiceImpl{
		streamManager:   streamManager,
		streams:         make(map[string]*Stream),
		mediamtxConfig:  mediamtxConfigPath,
		encoderSelector: selector,
	}
}

// NewStreamServiceWithOBS creates a new stream service with OBS monitoring integration
func NewStreamServiceWithOBS(streamManager *streamconfig.StreamManager, mediamtxConfigPath string,
	obsIntegration func(string, string, string) error, obsRemoval func(string) error) StreamService {
	// Create default encoder selector using stream manager for backward compatibility
	storage := valManager.NewStreamStorage(streamManager)
	vm := valManager.NewManager(storage)
	if err := vm.LoadValidation(); err != nil {
		log.Printf("Failed to load validation data: %v", err)
	}
	selector := encoders.NewDefaultSelector(vm)

	return &StreamServiceImpl{
		streamManager:   streamManager,
		streams:         make(map[string]*Stream),
		mediamtxConfig:  mediamtxConfigPath,
		obsIntegration:  obsIntegration,
		obsRemoval:      obsRemoval,
		encoderSelector: selector,
	}
}

// NewStreamServiceWithSelector creates a new stream service with custom encoder selector
func NewStreamServiceWithSelector(streamManager *streamconfig.StreamManager, mediamtxConfigPath string,
	selector encoders.Selector, obsIntegration func(string, string, string) error, obsRemoval func(string) error) StreamService {
	return &StreamServiceImpl{
		streamManager:   streamManager,
		streams:         make(map[string]*Stream),
		mediamtxConfig:  mediamtxConfigPath,
		obsIntegration:  obsIntegration,
		obsRemoval:      obsRemoval,
		encoderSelector: selector,
	}
}

// CreateStream creates a new video stream
func (s *StreamServiceImpl) CreateStream(ctx context.Context, params StreamCreateParams) (*Stream, error) {
	// Validate device ID
	devicePath := s.resolveDeviceID(params.DeviceID)
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

	// Convert generic codec (h264/h265) to optimal encoder with settings
	var encoder string
	var globalArgs []string
	var videoFilters string
	var encoderParams map[string]string
	var preset string

	// Get the optimal encoder based on the requested codec
	if params.Codec == "h264" || params.Codec == "h265" {
		// Convert string to CodecType
		var codecType encoders.CodecType
		if params.Codec == "h265" {
			codecType = encoders.CodecH265
		} else {
			codecType = encoders.CodecH264
		}

		// Use the encoder selector to find the best available encoder
		// Pass the input format and quality params
		optimalEncoder, settings, err := s.encoderSelector.SelectEncoder(codecType, params.InputFormat, qualityParams)
		if err != nil {
			log.Printf("Failed to get optimal encoder for %s: %v", codecType, err)
			// Fallback to default
			encoder = "libx264"
		} else {
			encoder = optimalEncoder
			if settings != nil {
				globalArgs = settings.GlobalArgs
				videoFilters = settings.VideoFilters
				encoderParams = settings.OutputParams // Now includes quality params merged
			}
			log.Printf("Selected %s encoder %s with hardware acceleration", codecType, encoder)
		}

		// Validate the selected encoder
		if err := s.encoderSelector.ValidateEncoder(encoder); err != nil {
			return nil, NewStreamError(ErrCodeInvalidParams,
				fmt.Sprintf("encoder validation failed: %v", err), err)
		}

		// Set preset for software encoders
		if strings.Contains(encoder, "libx") {
			preset = "fast"
		}
	} else {
		// Direct encoder specification (not a generic codec)
		encoder = params.Codec
		// For direct encoder specification, validate it's in the working list
		if err := s.encoderSelector.ValidateEncoder(encoder); err != nil {
			return nil, NewStreamError(ErrCodeInvalidParams,
				fmt.Sprintf("encoder validation failed: %v", err), err)
		}
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

	// Extract bitrate from encoderParams for display purposes only
	// The actual bitrate is in EncoderParams["b:v"], not in the legacy Bitrate field
	displayBitrate := ""
	if encoderParams != nil {
		if bv, ok := encoderParams["b:v"]; ok {
			displayBitrate = bv
		}
	}
	if displayBitrate == "" {
		displayBitrate = "2M" // Default for display
	}

	// Create stream configuration with FFmpeg section
	streamConfigTOML := streamconfig.StreamConfig{
		ID:      streamID,
		Name:    streamID,
		Device:  params.DeviceID, // Store stable device ID
		Enabled: true,
		FFmpeg: streamconfig.FFmpegConfig{
			InputFormat:   params.InputFormat,
			Resolution:    resolution,
			FPS:           fps,
			Encoder:       encoder,
			Preset:        preset,
			Bitrate:       "", // Never set the legacy Bitrate field when using EncoderParams
			GlobalArgs:    globalArgs,
			VideoFilters:  videoFilters,
			EncoderParams: encoderParams,
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
	if s.streamManager != nil {
		if err := s.streamManager.AddStream(streamConfigTOML); err != nil {
			log.Printf("Warning: Failed to save stream to TOML config: %v", err)
		} else {
			log.Printf("Saved stream %s to persistent TOML config", streamID)

			// Regenerate MediaMTX config from TOML with fresh socket paths
			// Stream is now in memory so updateSocketPaths will find it
			socketPaths, err := SyncAllStreamsToMediaMTX(s.streamManager, s.mediamtxConfig)
			if err != nil {
				log.Printf("Warning: Failed to sync MediaMTX config: %v", err)
			} else {
				log.Printf("Synchronized MediaMTX config after adding stream %s", streamID)
				// Update OBS collectors with new socket paths
				s.updateSocketPaths(socketPaths)
			}
		}
	}

	// Get the initialized stream from memory
	s.streamsMutex.RLock()
	stream := s.streams[streamID]
	s.streamsMutex.RUnlock()

	log.Printf("Created MediaMTX stream: %s for device %s", streamID, params.DeviceID)
	log.Printf("WebRTC URL: %s", stream.WebRTCURL)
	log.Printf("RTSP URL: %s", stream.RTSPURL)

	return stream, nil
}

// DeleteStream deletes an existing stream
func (s *StreamServiceImpl) DeleteStream(ctx context.Context, streamID string) error {
	// Check if stream exists
	s.streamsMutex.RLock()
	_, exists := s.streams[streamID]
	s.streamsMutex.RUnlock()

	if !exists {
		return NewStreamError(ErrCodeStreamNotFound,
			fmt.Sprintf("stream %s not found", streamID), nil)
	}

	// Remove from memory
	s.streamsMutex.Lock()
	delete(s.streams, streamID)
	s.streamsMutex.Unlock()

	// Stop OBS monitoring if available
	if s.obsRemoval != nil {
		if err := s.obsRemoval(streamID); err != nil {
			log.Printf("Warning: Failed to stop OBS monitoring for stream %s: %v", streamID, err)
		} else {
			log.Printf("Stopped OBS monitoring for stream %s", streamID)
		}
	}

	// Remove from persistent TOML config
	if s.streamManager != nil {
		if err := s.streamManager.RemoveStream(streamID); err != nil {
			log.Printf("Warning: Failed to remove stream from TOML config: %v", err)
		} else {
			log.Printf("Removed stream %s from persistent TOML config", streamID)

			// Regenerate MediaMTX config from TOML with fresh socket paths
			socketPaths, err := SyncAllStreamsToMediaMTX(s.streamManager, s.mediamtxConfig)
			if err != nil {
				log.Printf("Warning: Failed to sync MediaMTX config after deletion: %v", err)
			} else {
				log.Printf("Synchronized MediaMTX config after deleting stream %s", streamID)
				// Update OBS collectors with new socket paths
				s.updateSocketPaths(socketPaths)
			}
		}
	}

	log.Printf("Deleted stream: %s", streamID)
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

// resolveDeviceID maps stable device IDs to current device paths
func (s *StreamServiceImpl) resolveDeviceID(deviceID string) string {
	detector := devices.NewDetector()
	devicePath, err := detector.GetDevicePathByID(deviceID)
	if err != nil {
		log.Printf("Error resolving device ID %s: %v", deviceID, err)
		return ""
	}
	return devicePath
}

// LoadStreamsFromConfig loads existing streams from TOML config into memory
func (s *StreamServiceImpl) LoadStreamsFromConfig() error {
	if s.streamManager == nil {
		return fmt.Errorf("stream manager not initialized")
	}

	streams := s.streamManager.GetStreams()
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

	// Regenerate MediaMTX config from all loaded streams with fresh socket paths
	socketPaths, err := SyncAllStreamsToMediaMTX(s.streamManager, s.mediamtxConfig)
	if err != nil {
		log.Printf("Warning: Failed to sync MediaMTX config at startup: %v", err)
	} else {
		log.Printf("Synchronized MediaMTX config with %d streams at startup", streamCount)
		// Update OBS collectors with new socket paths
		s.updateSocketPaths(socketPaths)
	}
	return nil
}

// InitializeStream initializes a single stream with all integrations
func (s *StreamServiceImpl) InitializeStream(streamConfig streamconfig.StreamConfig) error {
	// Extract bitrate from encoder params for display purposes
	displayBitrate := streamConfig.FFmpeg.Bitrate // Use legacy field if set
	if displayBitrate == "" && streamConfig.FFmpeg.EncoderParams != nil {
		// Try to get from encoder params if legacy field is empty
		if bv, ok := streamConfig.FFmpeg.EncoderParams["b:v"]; ok {
			displayBitrate = bv
		}
	}
	if displayBitrate == "" {
		displayBitrate = "2M" // Default for display
	}

	// Create stream entity from config
	stream := &Stream{
		ID:        streamConfig.ID,
		DeviceID:  streamConfig.Device,
		Codec:     streamConfig.FFmpeg.Encoder, // Use the actual encoder from FFmpeg config
		Bitrate:   displayBitrate,              // Display bitrate extracted from encoder params or legacy field
		StartTime: streamConfig.CreatedAt,
		WebRTCURL: mediamtx.GetWebRTCURL(streamConfig.ID),
		RTSPURL:   mediamtx.GetRTSPURL(streamConfig.ID),
	}

	// Store the stream in memory - only lock for the write
	s.streamsMutex.Lock()
	s.streams[streamConfig.ID] = stream
	s.streamsMutex.Unlock()

	// OBS monitoring will be initialized when socket paths are generated during MediaMTX sync
	log.Printf("Initialized stream %s (device: %s, encoder: %s)", streamConfig.ID, streamConfig.Device, streamConfig.FFmpeg.Encoder)
	return nil
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
