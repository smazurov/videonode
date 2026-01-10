package streams

import (
	"context"
	"fmt"
	"time"

	"github.com/smazurov/videonode/internal/metrics/collectors"
)

// LoadStreamsFromConfig loads existing streams from TOML config into memory.
// Called only at startup - runtime management is via CRUD APIs.
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

	// Start all stream processes via process manager
	if s.processManager != nil {
		if err := s.processManager.StartAll(); err != nil {
			s.logger.Warn("Some streams failed to start", "error", err)
		}
	}

	return nil
}

// InitializeStream initializes a single stream with all integrations.
func (s *service) InitializeStream(streamConfig StreamSpec) error {
	socketPath := getSocketPath(streamConfig.ID)

	// Create and start metrics collector for this stream
	ffmpegCollector := collectors.NewFFmpegCollector(socketPath, streamConfig.ID)
	if err := ffmpegCollector.Start(context.Background()); err != nil {
		s.logger.Warn("Failed to start metrics collector for stream", "stream_id", streamConfig.ID, "error", err)
	}

	// Create stream runtime state
	stream := &Stream{
		ID:             streamConfig.ID,
		Enabled:        false, // Runtime state, set by device monitoring
		StartTime:      time.Now(),
		ProgressSocket: socketPath,
		Collector:      ffmpegCollector,
	}

	// Store the stream in memory
	s.streamsMutex.Lock()
	s.streams[streamConfig.ID] = stream
	s.streamsMutex.Unlock()

	s.logger.Info("Initialized stream", "stream_id", streamConfig.ID, "device", streamConfig.Device, "codec", streamConfig.FFmpeg.Codec)
	return nil
}
