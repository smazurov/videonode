package streams

import (
	"errors"
	"fmt"

	"github.com/smazurov/videonode/internal/logging"
	"github.com/smazurov/videonode/internal/streams/pipeline"
)

// ReplayV2Entities applies every persisted v2 entity onto the supervised
// pipeline at startup. When pipelineEnabled=true, all entities are fully
// applied (processes spawned). When false, sources and composers are
// registered in the pipeline's in-memory registry (for upstream-ref
// resolution) but no processes are spawned.
//
// Order matters: downstream stages reference upstream ids via
// resolveUpstream, so producers must be in the registry before composers,
// and composers before streams.
func ReplayV2Entities(store EntityStore, pipe *pipeline.Pipeline, pipelineEnabled bool) error {
	logger := logging.GetLogger("startup")
	if store == nil || pipe == nil {
		return nil
	}
	var errs []error

	sources := store.ListSourceEntities()
	for _, src := range sources {
		var err error
		if pipelineEnabled {
			err = pipe.ApplySource(src)
		} else {
			err = pipe.RegisterSource(src)
		}
		if err != nil {
			logger.Warn("ReplayV2Entities: source replay failed", "source_id", src.ID, "error", err)
			errs = append(errs, fmt.Errorf("source %s: %w", src.ID, err))
		}
	}

	composers := store.ListComposerEntities()
	for _, c := range composers {
		var err error
		if pipelineEnabled {
			err = pipe.ApplyComposer(c)
		} else {
			err = pipe.RegisterComposer(c)
		}
		if err != nil {
			logger.Warn("ReplayV2Entities: composer replay failed", "composer_id", c.ID, "error", err)
			errs = append(errs, fmt.Errorf("composer %s: %w", c.ID, err))
		}
	}

	v2streams := store.ListPipelineStreams()
	appliedStreams := 0
	if pipelineEnabled {
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
		"pipeline_enabled", pipelineEnabled)

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}
