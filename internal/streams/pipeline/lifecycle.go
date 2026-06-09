package pipeline

import (
	"context"
	"errors"
	"fmt"

	"github.com/smazurov/videonode/internal/logging"
	"github.com/smazurov/videonode/internal/process"
)

// StartAll applies every entity in dependency order (sources → composers →
// streams), skipping non-idle ones and logging per-entity failures instead of
// propagating them. Returns the failure count. On a fresh (empty) pool nothing
// is skipped, so this is also the startup hydration path.
func (p *Pipeline) StartAll(ctx context.Context, sources []Source, composers []Composer, streams []Stream) int {
	p.logger.Info("Starting entities",
		logging.KeySources, len(sources), logging.KeyComposers, len(composers), logging.KeyStreams, len(streams))

	pool := p.Pool()
	var failed int

	for _, src := range sources {
		if ctx.Err() != nil {
			return failed
		}
		if pool.GetStatus(SourcePoolKey(src.ID)).State != process.StateIdle {
			continue
		}
		if err := p.ApplySource(src); err != nil {
			failed++
			p.logger.Error("ApplySource failed", logging.KeySourceID, src.ID, logging.KeyError, err)
		}
	}

	for _, c := range composers {
		if ctx.Err() != nil {
			return failed
		}
		if pool.GetStatus(ComposerPoolKey(c.ID)).State != process.StateIdle {
			continue
		}
		if err := p.ApplyComposer(c); err != nil {
			failed++
			p.logger.Error("ApplyComposer failed", logging.KeyComposerID, c.ID, logging.KeyError, err)
		}
	}

	for _, st := range streams {
		if ctx.Err() != nil {
			return failed
		}
		if pool.GetStatus(EncoderIDFor(st.ID)).State != process.StateIdle {
			continue
		}
		if err := p.ApplyStream(st); err != nil {
			failed++
			p.logger.Error("ApplyStream failed", logging.KeyStreamID, st.ID, logging.KeyError, err)
		}
	}

	if failed > 0 {
		p.logger.Warn("Start complete with failures", logging.KeyFailed, failed)
	} else {
		p.logger.Info("Start complete")
	}
	return failed
}

// StopAll stops every supervised process in reverse dependency order
// (streams → composers → sources), preserving registry entries. Per-entity
// failures are logged and joined into the returned error.
func (p *Pipeline) StopAll(sources []Source, composers []Composer, streams []Stream) error {
	var errs []error

	for _, st := range streams {
		if err := p.StopEncoder(st.ID); err != nil {
			p.logger.Error("StopEncoder failed", logging.KeyStreamID, st.ID, logging.KeyError, err)
			errs = append(errs, fmt.Errorf("stream %s: %w", st.ID, err))
		}
	}
	for _, c := range composers {
		if err := p.StopComposer(c.ID); err != nil {
			p.logger.Error("StopComposer failed", logging.KeyComposerID, c.ID, logging.KeyError, err)
			errs = append(errs, fmt.Errorf("composer %s: %w", c.ID, err))
		}
	}
	for _, src := range sources {
		if err := p.StopSource(src.ID); err != nil {
			p.logger.Error("StopSource failed", logging.KeySourceID, src.ID, logging.KeyError, err)
			errs = append(errs, fmt.Errorf("source %s: %w", src.ID, err))
		}
	}

	return errors.Join(errs...)
}
