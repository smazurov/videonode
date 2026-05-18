package streams

import (
	"context"
	"fmt"
	"time"

	"github.com/smazurov/videonode/internal/metrics/collectors"
)

// LoadStreamsFromConfig loads streams from TOML config into memory at startup.
func (s *service) LoadStreamsFromConfig() error {
	if s.store == nil {
		return fmt.Errorf("repository not initialized")
	}

	if err := s.store.Load(); err != nil {
		return fmt.Errorf("failed to load streams configuration: %w", err)
	}

	streams := s.store.GetAllStreams()
	s.logger.Info("Loaded streams from configuration", "count", len(streams))

	for _, streamConfig := range streams {
		if err := s.InitializeStream(streamConfig); err != nil {
			s.logger.Warn("Failed to initialize stream", "stream_id", streamConfig.ID, "error", err)
			continue
		}
	}

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

	ffmpegCollector := collectors.NewFFmpegCollector(socketPath, streamConfig.ID)
	if err := ffmpegCollector.Start(context.Background()); err != nil {
		s.logger.Warn("Failed to start metrics collector for stream", "stream_id", streamConfig.ID, "error", err)
	}

	// Canvases default to engaged because they have no hardware device to wait
	// on; the dormant state from a prior release is persisted on the spec
	// (Canvas.Enabled=false) so a restart keeps the user's last toggle.
	// Single streams default to Enabled=false and are flipped true by either
	// the create path (after device validation) or device discovery.
	enabled := streamConfig.Canvas != nil && streamConfig.Canvas.IsEngaged()

	stream := &Stream{
		ID:             streamConfig.ID,
		Enabled:        enabled,
		StartTime:      time.Now(),
		ProgressSocket: socketPath,
		Collector:      ffmpegCollector,
	}

	if streamConfig.Canvas != nil {
		stream.InputsEnabled = make(map[string]bool, len(streamConfig.Canvas.SourceStreams))
		for _, srcID := range streamConfig.Canvas.SourceStreams {
			stream.InputsEnabled[srcID] = false
		}
	}

	s.streamsMutex.Lock()
	s.streams[streamConfig.ID] = stream
	s.streamsMutex.Unlock()

	s.logger.Info("Initialized stream", "stream_id", streamConfig.ID, "device", streamConfig.Device, "codec", streamConfig.FFmpeg.Codec)
	return nil
}
