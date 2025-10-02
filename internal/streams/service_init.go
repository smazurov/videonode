package streams

import (
	"fmt"
	"time"

	"github.com/smazurov/videonode/internal/mediamtx"
	"github.com/smazurov/videonode/internal/obs/collectors"
)

// LoadStreamsFromConfig loads existing streams from TOML config into memory
// This is called only at startup - runtime management is via CRUD APIs
// Not part of StreamService interface - use CRUD APIs for runtime management
func (s *service) LoadStreamsFromConfig() error {
	if s.store == nil {
		return fmt.Errorf("repository not initialized")
	}

	// Load the configuration from file
	if err := s.store.Load(); err != nil {
		return fmt.Errorf("failed to load streams configuration: %w", err)
	}

	streams := s.store.GetAllStreams()
	// No lock needed here - InitializeStream handles its own locking

	for _, streamConfig := range streams {
		// Initialize ALL streams regardless of enabled state
		// Enabled state is runtime-only and controlled by device monitoring
		if err := s.InitializeStream(streamConfig); err != nil {
			s.logger.Warn("Failed to initialize stream", "stream_id", streamConfig.ID, "error", err)
			continue
		}
	}

	s.logger.Info("Loaded streams from configuration")

	// Sync all streams to MediaMTX via API
	if err := s.mediamtxClient.SyncAll(); err != nil {
		s.logger.Warn("Failed to sync MediaMTX at startup", "error", err)
	}

	return nil
}

// InitializeStream initializes a single stream with all integrations
func (s *service) InitializeStream(streamConfig StreamSpec) error {
	// Create stream runtime state
	// Enabled defaults to false and will be set by device monitoring
	stream := &Stream{
		ID:             streamConfig.ID,
		Enabled:        false,      // Runtime state, set by device monitoring
		StartTime:      time.Now(), // Track when loaded into memory
		ProgressSocket: getSocketPath(streamConfig.ID),
	}

	// Store the stream in memory - only lock for the write
	s.streamsMutex.Lock()
	s.streams[streamConfig.ID] = stream
	s.streamsMutex.Unlock()

	// Initialize OBS monitoring
	if s.obsManager != nil {
		socketPath := getSocketPath(streamConfig.ID)
		ffmpegCollector := collectors.NewFFmpegCollector(socketPath, "", streamConfig.ID)
		if err := s.obsManager.AddCollector(ffmpegCollector); err != nil {
			s.logger.Warn("Failed to register OBS collector for stream", "stream_id", streamConfig.ID, "error", err)
		}
	}

	s.logger.Info("Initialized stream", "stream_id", streamConfig.ID, "device", streamConfig.Device, "codec", streamConfig.FFmpeg.Codec)
	return nil
}

// setupMediaMTXClient creates and initializes the MediaMTX API client
func setupMediaMTXClient(service *service) *mediamtx.Client {
	client := mediamtx.NewClient("http://localhost:9997", service.getProcessedStreamsForSync)
	client.StartHealthMonitor()
	return client
}