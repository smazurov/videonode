package streams

import (
	"errors"
	"fmt"

	"github.com/smazurov/videonode/internal/logging"
	"github.com/smazurov/videonode/internal/streams/pipeline"
)

// ReplayV2Entities applies every persisted v2 entity onto the supervised
// pipeline at startup. Sources and composers always replay so their
// registries are populated and stream upstream-ref validation works.
// Streams are replayed only when streamsEnabled=true (the persisted
// pipeline master switch): when the switch is off, the encoder stage
// stays uncached and the user has to flip it on (or POST /api/pipeline/start)
// before any encoder spawns.
//
// Order matters: downstream stages reference upstream ids via
// resolveUpstream, so producers must be in the registry before composers,
// and composers before streams.
func ReplayV2Entities(store EntityStore, pipe *pipeline.Pipeline, streamsEnabled bool) error {
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
	appliedStreams := 0
	if streamsEnabled {
		for _, st := range v2streams {
			if err := pipe.ApplyStream(st); err != nil {
				logger.Warn("ReplayV2Entities: ApplyStream failed", "stream_id", st.ID, "error", err)
				errs = append(errs, fmt.Errorf("stream %s: %w", st.ID, err))
				continue
			}
			appliedStreams++
		}
	}

	logger.Info("Replayed v2 entities",
		"sources", len(sources),
		"composers", len(composers),
		"streams_persisted", len(v2streams),
		"streams_applied", appliedStreams,
		"streams_enabled", streamsEnabled)

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}
