package streams

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	streamconfig "github.com/smazurov/videonode/internal/config"
	"github.com/smazurov/videonode/internal/encoders"
	"github.com/smazurov/videonode/internal/mediamtx"
	"github.com/smazurov/videonode/v4l2_detector"
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
	streamManager  *streamconfig.StreamManager
	streams        map[string]*Stream
	streamsMutex   sync.RWMutex
	mediamtxConfig string
	obsIntegration func(string, string, string) error // Function to add OBS monitoring
	obsRemoval     func(string) error                 // Function to remove OBS monitoring
}

// NewStreamService creates a new stream service
func NewStreamService(streamManager *streamconfig.StreamManager, mediamtxConfigPath string) StreamService {
	return &StreamServiceImpl{
		streamManager:  streamManager,
		streams:        make(map[string]*Stream),
		mediamtxConfig: mediamtxConfigPath,
	}
}

// NewStreamServiceWithOBS creates a new stream service with OBS monitoring integration
func NewStreamServiceWithOBS(streamManager *streamconfig.StreamManager, mediamtxConfigPath string,
	obsIntegration func(string, string, string) error, obsRemoval func(string) error) StreamService {
	return &StreamServiceImpl{
		streamManager:  streamManager,
		streams:        make(map[string]*Stream),
		mediamtxConfig: mediamtxConfigPath,
		obsIntegration: obsIntegration,
		obsRemoval:     obsRemoval,
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

	// Load MediaMTX configuration
	config, err := mediamtx.LoadFromFile(s.mediamtxConfig)
	if err != nil {
		return nil, NewStreamError(ErrCodeMediaMTXError,
			"failed to load MediaMTX configuration", err)
	}

	// Generate socket path with timestamp to avoid conflicts
	timestamp := time.Now().Unix()
	socketPath := fmt.Sprintf("/tmp/ffmpeg-progress-%s-%d.sock", streamID, timestamp)

	// Build resolution string
	var resolution string
	if params.Width != nil && params.Height != nil && *params.Width > 0 && *params.Height > 0 {
		resolution = fmt.Sprintf("%dx%d", *params.Width, *params.Height)
	}

	// Build framerate string
	var fps string
	if params.Framerate != nil && *params.Framerate > 0 {
		fps = fmt.Sprintf("%d", *params.Framerate)
	}

	// Map API codec to FFmpeg encoder
	encoderConfig, err := encoders.MapAPICodec(params.Codec)
	if err != nil {
		return nil, NewStreamError(ErrCodeInvalidCodec,
			fmt.Sprintf("failed to map codec %s: %v", params.Codec, err), nil)
	}

	// Add stream to MediaMTX configuration
	streamConfig := mediamtx.StreamConfig{
		DevicePath:     devicePath,
		Resolution:     resolution,
		FPS:            fps,
		Codec:          encoderConfig.EncoderName,
		ProgressSocket: socketPath,
		GlobalArgs:     encoderConfig.Settings.GlobalArgs,
		EncoderParams:  encoderConfig.Settings.OutputParams,
		VideoFilters:   encoderConfig.Settings.VideoFilters,
	}
	err = config.AddStream(streamID, streamConfig)
	if err != nil {
		return nil, NewStreamError(ErrCodeMediaMTXError,
			"failed to configure stream", err)
	}

	// Write updated configuration to file
	err = config.WriteToFile(s.mediamtxConfig)
	if err != nil {
		return nil, NewStreamError(ErrCodeMediaMTXError,
			"failed to save MediaMTX configuration", err)
	}

	log.Printf("Added stream %s to MediaMTX config for device %s", streamID, devicePath)

	// Save to persistent TOML config
	if s.streamManager != nil {
		streamConfigTOML := streamconfig.StreamConfig{
			ID:             streamID,
			Name:           streamID,
			Device:         params.DeviceID, // Store stable device ID
			Enabled:        true,
			Resolution:     resolution,
			FPS:            fps,
			Codec:          params.Codec,
			ProgressSocket: socketPath,
		}

		if err := s.streamManager.AddStream(streamConfigTOML); err != nil {
			log.Printf("Warning: Failed to save stream to TOML config: %v", err)
		} else {
			log.Printf("Saved stream %s to persistent TOML config", streamID)
		}
	}

	// Create stream entity
	stream := &Stream{
		ID:        streamID,
		DeviceID:  params.DeviceID,
		Codec:     params.Codec,
		StartTime: time.Now(),
		WebRTCURL: mediamtx.GetWebRTCURL(streamID),
		RTSPURL:   mediamtx.GetRTSPURL(streamID),
	}

	// Store the stream in memory
	s.streamsMutex.Lock()
	s.streams[streamID] = stream
	s.streamsMutex.Unlock()

	// Start OBS monitoring if available
	if s.obsIntegration != nil {
		logPath := "" // No log path for now, socket monitoring only
		if err := s.obsIntegration(streamID, socketPath, logPath); err != nil {
			log.Printf("Warning: Failed to start OBS monitoring for stream %s: %v", streamID, err)
		} else {
			log.Printf("Started OBS monitoring for stream %s on socket: %s", streamID, socketPath)
		}
	}

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

	// Load MediaMTX configuration
	config, err := mediamtx.LoadFromFile(s.mediamtxConfig)
	if err != nil {
		return NewStreamError(ErrCodeMediaMTXError,
			"failed to load MediaMTX configuration", err)
	}

	// Remove stream from MediaMTX configuration
	config.RemoveStream(streamID)

	// Write updated configuration to file
	err = config.WriteToFile(s.mediamtxConfig)
	if err != nil {
		return NewStreamError(ErrCodeMediaMTXError,
			"failed to save MediaMTX configuration", err)
	}

	log.Printf("Removed stream %s from MediaMTX config", streamID)

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
	devices, err := v4l2_detector.FindDevices()
	if err != nil {
		log.Printf("Error finding devices for resolution: %v", err)
		return ""
	}

	for _, device := range devices {
		if device.DeviceId == deviceID {
			return device.DevicePath
		}
	}
	return ""
}

// LoadStreamsFromConfig loads existing streams from TOML config into memory
func (s *StreamServiceImpl) LoadStreamsFromConfig() error {
	if s.streamManager == nil {
		return fmt.Errorf("stream manager not initialized")
	}

	streams := s.streamManager.GetStreams()
	s.streamsMutex.Lock()
	defer s.streamsMutex.Unlock()

	for _, streamConfig := range streams {
		// Only load enabled streams
		if !streamConfig.Enabled {
			continue
		}

		// Create stream entity from config
		stream := &Stream{
			ID:        streamConfig.ID,
			DeviceID:  streamConfig.Device,
			Codec:     streamConfig.Codec,
			StartTime: streamConfig.CreatedAt, // Use creation time as start time
			WebRTCURL: mediamtx.GetWebRTCURL(streamConfig.ID),
			RTSPURL:   mediamtx.GetRTSPURL(streamConfig.ID),
		}

		// Store the stream in memory
		s.streams[streamConfig.ID] = stream
		log.Printf("Loaded stream %s from config (device: %s)", streamConfig.ID, streamConfig.Device)
	}

	log.Printf("Loaded %d streams from configuration", len(s.streams))
	return nil
}
