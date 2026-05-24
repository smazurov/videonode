package pipeline

import (
	"errors"
	"log/slog"

	"github.com/smazurov/videonode/internal/process"
)

// Kind enumerates the three supervised process kinds. Replaces the
// legacy `producer:<id>` string-prefix discriminator in process.Pool —
// callers can now switch on a typed Kind instead of string-matching.
type Kind int

// Kind values: Unknown is the zero value reserved for unset; the other
// three identify each supervised process kind for log-module selection
// and process-pool discrimination.
const (
	KindUnknown  Kind = iota
	KindProducer      // videonode-source — per unique device, refcounted
	KindComposer      // videonode-composer — per stream when N>1 or effects
	KindEncoder       // vn-sink | ffmpeg — per stream, always
)

// String returns the lowercased name used as the journald MODULE
// attribute for this stage's logs.
func (k Kind) String() string {
	switch k {
	case KindProducer:
		return "producer"
	case KindComposer:
		return "composer"
	case KindEncoder:
		return "encoder"
	default:
		return "unknown"
	}
}

// ErrRequiresRestart is returned by Stage.Reconfigure when a spec change
// can't be applied via the stage's live control plane and requires a
// full process restart. The Pipeline catches this and orchestrates the
// restart, preserving sibling stages where possible.
var ErrRequiresRestart = errors.New("pipeline: stage reconfigure requires restart")

// Stage is the unified contract every supervised process implements.
// One instance per running process. The Pipeline owns the lifecycle and
// drives reconfigure; Stage implementations encapsulate the per-kind
// command construction, log parsing, attribute tagging, and live-update
// behavior.
type Stage interface {
	// ID is the process.Pool key. Stable across restarts of the same
	// logical stage (e.g. "producer:hdmi0" or "composer:my-stream").
	ID() string

	// Kind tags the stage for log-module selection and process-pool
	// discrimination. Stable for the stage's lifetime.
	Kind() Kind

	// StreamID is the user-facing stream this stage participates in.
	// For producers shared across streams, returns the empty string —
	// producer logs are attributed to the device, not any one stream.
	StreamID() string

	// Command returns the argv + env for the next process.Pool spawn.
	// Called fresh each time the pool restarts the stage; if the stage
	// holds dynamic config (e.g. layout, effects), it must reflect the
	// current desired state.
	Command() (argv []string, env []string, err error)

	// LogParser extracts (level, msg) from a single stderr line. All
	// three stage kinds today emit `[level] msg` (vn::log helpers on
	// the C++ side, ffmpeg's native bracket prefix on the encoder side);
	// returning ffmpeg.ParseLogLevel is the right answer for now.
	LogParser() process.LogParser

	// LogAttrs returns the slog attributes attached to every line of
	// this stage's stderr. At minimum: stream_id (when applicable),
	// stage_instance (the pool key), device (producer), slot (composer).
	LogAttrs() []slog.Attr

	// Reconfigure attempts to apply a new spec without restarting. The
	// stage may live-update via its control plane (e.g. composer's
	// SetLayout RPC) and return nil; if the change can't be applied
	// live it returns ErrRequiresRestart and the Pipeline orchestrates
	// the restart.
	Reconfigure(spec any) error
}
