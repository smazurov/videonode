package services

import (
	"github.com/smazurov/videonode/internal/api"
	"github.com/smazurov/videonode/internal/logging"
	"github.com/smazurov/videonode/internal/streams"
	"github.com/smazurov/videonode/internal/streams/pipeline"
)

func rejectStream(err error) error {
	return &api.StreamInvalidError{Message: "pipeline rejected stream: " + err.Error()}
}

func rejectSource(err error) error {
	return &api.SourceInvalidError{Message: "pipeline rejected source: " + err.Error()}
}

// switchEnabled reports the daemon-wide pipeline master switch. A nil switch
// (tests, persistence-only wiring) reads as enabled.
func switchEnabled(psw PipelineSwitch) bool {
	if psw == nil {
		return true
	}
	return psw.GetPipeline().Enabled
}

// applyOrRollback mirrors a persisted change onto the pipeline via apply. If
// apply fails it runs rollback to undo the persisted write, logs when that
// rollback itself also fails, and returns rejFor(applyErr) so the caller
// surfaces the pipeline's rejection. Returns nil on success.
func applyOrRollback(apply, rollback func() error, logger logging.Logger, op string, rejFor func(error) error, kv ...any) error {
	err := apply()
	if err == nil {
		return nil
	}
	if rbErr := rollback(); rbErr != nil {
		logger.Error(op+": rollback after pipeline failure also failed",
			append(append([]any(nil), kv...), logging.KeyApplyError, err, logging.KeyRollbackError, rbErr)...)
	}
	return rejFor(err)
}

// encoderRebuilder is the slice of the pipeline rebuildEncodersForUpstream needs.
type encoderRebuilder interface {
	RebuildStreamEncoder(pipeline.Stream) error
}

// rebuildEncodersForUpstream bounces the encoder of every stream whose upstream
// is ref, so a producer geometry change reaches each consumer's launch-time
// ffmpeg `-s`. Best-effort: logs and continues on per-stream failure.
func rebuildEncodersForUpstream(pipe encoderRebuilder, store streams.EntityStore, logger logging.Logger, ref string) {
	if pipe == nil {
		return
	}
	for _, st := range store.ListPipelineStreams() {
		if st.Upstream != ref {
			continue
		}
		if err := pipe.RebuildStreamEncoder(st); err != nil {
			logger.Warn("rebuild dependent encoder failed", logging.KeyStreamID, st.ID, logging.KeyError, err)
		}
	}
}
