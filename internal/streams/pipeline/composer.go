package pipeline

import (
	"errors"
	"log/slog"
	"strconv"

	"github.com/smazurov/videonode/internal/ffmpeg"
	"github.com/smazurov/videonode/internal/process"
)

// ComposerStage is the per-stream videonode-composer process. One
// instance per stream when NeedsComposer(stream) is true (N>1 OR any
// effects). Reads N producer SCM sockets, GLES-composites onto a BGRA
// canvas, broadcasts the canvas dma-buf via SCM_RIGHTS to the encoder
// (--scm-out).
//
// Per-frame layout / effect / source-state config is daemon-pushed
// over the per-instance gRPC control plane (Composer.SetCanvas /
// SetSource / SetLayout / SetEffects / SetSourceState). The composer
// boot argv is intentionally bare; everything dynamic flows through
// the control plane post-spawn.
type ComposerStage struct {
	StreamID_  string // user-facing stream id
	BinaryPath string // path to videonode-composer binary
	DRMDevice  string // e.g. "/dev/dri/renderD130" on the rig
	CanvasFPS  int    // pre-ready tick rate (daemon's SetCanvas wins once pushed)
	// GrpcUds is the per-instance UDS the composer binds. Required —
	// the GPU composer path needs the daemon's control plane (without
	// it, the composer renders solid black forever).
	GrpcUds string
}

// ComposerPoolKey returns the pool.Pool key for a stream's composer.
func ComposerPoolKey(streamID string) string {
	return "composer:" + streamID
}

// ComposerIDFor returns the stable composer-id the daemon tells the
// composer to identify as. Matches the convention used by the legacy
// canvas_processor_gpu.go composerIDFor.
func ComposerIDFor(streamID string) string {
	return streamID + "-composer"
}

// ID returns the stage's process.Pool key.
func (c *ComposerStage) ID() string { return ComposerPoolKey(c.StreamID_) }

// Kind reports this as a Composer stage.
func (c *ComposerStage) Kind() Kind { return KindComposer }

// StreamID returns the user-facing stream id.
func (c *ComposerStage) StreamID() string { return c.StreamID_ }

// SCMOutSocketPath returns the canvas output socket the composer binds
// (--scm-out). The encoder dials this via vn-sink.
func (c *ComposerStage) SCMOutSocketPath() string {
	return SCMSocketPathFor("composer-" + c.StreamID_)
}

// Command returns the videonode-composer argv. Always uses --scm-out
// post-rip (no stdout-pipe mode in production), so encoder restart
// doesn't kill the composer via EPIPE.
func (c *ComposerStage) Command() ([]string, []string, error) {
	if c.BinaryPath == "" {
		return nil, nil, errors.New("composer: BinaryPath is required")
	}
	if c.DRMDevice == "" {
		return nil, nil, errors.New("composer: DRMDevice is required")
	}
	if c.GrpcUds == "" {
		return nil, nil, errors.New("composer: GrpcUds is required (control plane mandatory)")
	}
	fps := c.CanvasFPS
	if fps <= 0 {
		fps = 30
	}
	argv := []string{
		c.BinaryPath,
		"--drm-device", c.DRMDevice,
		"--grpc-listen", c.GrpcUds,
		"--composer-id", ComposerIDFor(c.StreamID_),
		"--scm-out", c.SCMOutSocketPath(),
		"--target-fps", strconv.Itoa(fps),
	}
	return argv, nil, nil
}

// LogParser uses the ffmpeg parser — composer emits the same
// `[level] msg` format via vn::log helpers.
func (c *ComposerStage) LogParser() process.LogParser {
	return ffmpeg.ParseLogLevel
}

// LogAttrs tags composer logs with the user-facing stream id + the
// pool-key instance.
func (c *ComposerStage) LogAttrs() []slog.Attr {
	return []slog.Attr{
		slog.String("stream_id", c.StreamID_),
		slog.String("stage_instance", c.ID()),
	}
}

// Reconfigure: most composer config (canvas dims, slot bindings,
// layout, per-source effects + state) lives on the control plane and
// applies hot via the existing pipelinectl RPCs. The Pipeline's
// reconfigure path is responsible for routing the diff to the right
// RPC; this Stage.Reconfigure returns nil to signal "hot updates
// available — caller should push via control plane." Truly
// non-hot-applicable changes (DRM device path, GrpcUds path) return
// ErrRequiresRestart.
//
// For the initial cut we accept anything as "control-plane handled" —
// the Pipeline will refine the spec-diff routing in a follow-up
// commit. Callers that explicitly want restart pass a non-nil spec
// with a sentinel; today we don't surface that.
func (c *ComposerStage) Reconfigure(_ any) error { return nil }
