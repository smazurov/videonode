package streams

import (
	"fmt"
	"time"

	"github.com/smazurov/videonode/internal/obs/collectors"
)

// LoadStreamsFromConfig loads existing streams from TOML config into memory.
// Called only at startup - runtime management is via CRUD APIs.
func (s *service) LoadStreamsFromConfig() error {
	if s.store == nil {
		return fmt.Errorf("repository not initialized")
	}

	// Load the configuration from file
	if loadErr := s.store.Load(); loadErr != nil {
		return fmt.Errorf("failed to load streams configuration: %w", loadErr)
	}

	streams := s.store.GetAllStreams()
	// No lock needed here - InitializeStream handles its own locking

	for _, streamConfig := range streams {
		// Initialize ALL streams regardless of enabled state
		// Enabled state is runtime-only and controlled by device monitoring
		if initErr := s.InitializeStream(streamConfig); initErr != nil {
			s.logger.Warn("Failed to initialize stream", "stream_id", streamConfig.ID, "error", initErr)
			continue
		}
	}

	s.logger.Info("Loaded streams from configuration")

	// Start all stream processes via process manager
	if s.processManager != nil {
		if startErr := s.processManager.StartAll(); startErr != nil {
			s.logger.Warn("Some streams failed to start", "error", startErr)
		}
	}

	return nil
}

// InitializeStream initializes a single stream with all integrations.
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
		if addErr := s.obsManager.AddCollector(ffmpegCollector); addErr != nil {
			s.logger.Warn("Failed to register OBS collector for stream", "stream_id", streamConfig.ID, "error", addErr)
		}
	}

	s.logger.Info("Initialized stream", "stream_id", streamConfig.ID, "device", streamConfig.Device, "codec", streamConfig.FFmpeg.Codec)
	return nil
}
