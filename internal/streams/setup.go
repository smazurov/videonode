package streams

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/smazurov/videonode/internal/logging"
	"github.com/smazurov/videonode/internal/metrics/collectors"
	"github.com/smazurov/videonode/internal/streams/pipeline"
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
		if s.store.GetPipeline().Enabled {
			if err := s.processManager.StartAll(); err != nil {
				s.logger.Warn("Some streams failed to start", "error", err)
			}
		} else {
			s.logger.Info("Pipeline master switch is off; skipping auto-start")
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

// ReplayV2Entities applies every persisted v2 entity (sources → composers
// → streams) onto the supervised pipeline at startup. Without this the
// pipeline.stages map is empty after a daemon restart, so the first
// consumer attach falls into EnsureStreamReady → EnsureEncoder and
// errors with "no cached encoder stage" until the user re-PUTs each
// entity by hand. Order matters: downstream stages reference upstream
// ids via resolveUpstream, so producers must be in the registry before
// composers, and composers before streams.
func ReplayV2Entities(store EntityStore, pipe *pipeline.Pipeline) error {
	logger := logging.GetLogger("startup")
	if store == nil || pipe == nil {
		return nil
	}
	var errs []error

	sources := store.ListSourceEntities()
	for _, src := range sources {
		if err := pipe.ApplySource(src); err != nil {
			logger.Warn("ReplayV2Entities: ApplySource failed", "source_id", src.ID, "error", err)
			errs = append(errs, fmt.Errorf("source %s: %w", src.ID, err))
		}
	}

	composers := store.ListComposerEntities()
	for _, c := range composers {
		if err := pipe.ApplyComposer(c); err != nil {
			logger.Warn("ReplayV2Entities: ApplyComposer failed", "composer_id", c.ID, "error", err)
			errs = append(errs, fmt.Errorf("composer %s: %w", c.ID, err))
		}
	}

	v2streams := store.ListPipelineStreams()
	for _, st := range v2streams {
		if err := pipe.ApplyStream(st); err != nil {
			logger.Warn("ReplayV2Entities: ApplyStream failed", "stream_id", st.ID, "error", err)
			errs = append(errs, fmt.Errorf("stream %s: %w", st.ID, err))
		}
	}

	logger.Info("Replayed v2 entities",
		"sources", len(sources),
		"composers", len(composers),
		"streams", len(v2streams))

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}
