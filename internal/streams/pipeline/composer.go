package pipeline

import (
	"errors"
	"log/slog"
	"strconv"
	"time"

	"github.com/smazurov/videonode/internal/ffmpeg"
	"github.com/smazurov/videonode/internal/logging"
	"github.com/smazurov/videonode/internal/process"
)

// Composer is a daemon-managed GLES compositor keyed by stable id. One
// Composer per `videonode-composer` process; Streams reference it by id
// (`composer:<id>`). Multi-stream sharing is expressed by two Streams
// naming the same composer — they pay one GPU compose, two encodes.
//
// Inputs reference Sources via `source:<id>`. Layout is by-name (matches
// ComposerInput.Ref), not positional, so reordering inputs doesn't
// silently reshuffle the canvas.
type Composer struct {
	ID        string          `toml:"id" json:"id"`
	Canvas    CanvasDims      `toml:"canvas" json:"canvas"`
	Inputs    []ComposerInput `toml:"inputs" json:"inputs"`
	Layout    []LayoutSlot    `toml:"layout" json:"layout"`
	CreatedAt time.Time       `toml:"created_at" json:"created_at"`
	UpdatedAt time.Time       `toml:"updated_at" json:"updated_at"`
}

// CanvasDims is the output canvas size and render rate the composer
// renders to. Streams pulling from this composer encode at these
// dimensions and frame rate. FPS of zero falls back to DefaultCanvasFPS.
type CanvasDims struct {
	W   int `toml:"w" json:"w"`
	H   int `toml:"h" json:"h"`
	FPS int `toml:"fps,omitempty" json:"fps,omitempty"`
	// Background is the canvas clear color sources composite on top of, as
	// "#RRGGBB" or "#RRGGBBAA" hex. Empty persists as opaque black.
	Background string `toml:"background,omitempty" json:"background,omitempty"`
}

// DefaultCanvasFPS is the fallback canvas frame rate used when a
// composer's Canvas.FPS is zero (unset).
const DefaultCanvasFPS = 60

// ComposerInput binds a Source to this composer with an optional effect.
// Ref is `source:<id>`.
type ComposerInput struct {
	Ref    string  `toml:"ref" json:"ref"`
	Effect *Effect `toml:"effect,omitempty" json:"effect,omitempty"`
}

// CropConfig holds crop-mode positioning. Inner fields have no
// omitempty so zero values (valid "top-left") are always persisted.
type CropConfig struct {
	X     float64 `toml:"x" json:"x"`
	Y     float64 `toml:"y" json:"y"`
	Scale float64 `toml:"scale" json:"scale"`
}

// LayoutSlot positions one input on the canvas. Input is the matching
// ComposerInput.Ref (by name, not positional).
type LayoutSlot struct {
	Input           string      `toml:"input" json:"input"`
	X               int         `toml:"x" json:"x"`
	Y               int         `toml:"y" json:"y"`
	W               int         `toml:"w" json:"w"`
	H               int         `toml:"h" json:"h"`
	Rotation        int         `toml:"rotation,omitempty" json:"rotation,omitempty"`
	AspectRatioMode string      `toml:"aspect_ratio_mode,omitempty" json:"aspect_ratio_mode,omitempty"`
	Crop            *CropConfig `toml:"crop,omitempty" json:"crop,omitempty"`
}

// Effect is a per-input transformation applied by the composer. Today
// only "perspective" is implemented; new types tag-union via Type and
// extend with their own fields.
//
// SnapshotW/SnapshotH define the pixel-coordinate space the Corners
// quad is expressed in — typically the source's native resolution as
// captured by the snapshot UI. The composer normalizes corners to UV
// by dividing by these dims, so both must be > 0 for perspective.
type Effect struct {
	Type      string    `toml:"type" json:"type"`
	Corners   [4][2]int `toml:"corners,omitempty" json:"corners,omitempty"`
	SnapshotW int       `toml:"snapshot_w,omitempty" json:"snapshot_w,omitempty"`
	SnapshotH int       `toml:"snapshot_h,omitempty" json:"snapshot_h,omitempty"`
}

// ComposerStage is the per-Composer supervised `videonode-composer`
// process. Reads N source SCM sockets, GLES-composites onto a BGRA
// canvas, broadcasts the canvas dma-buf via SCM_RIGHTS (`--scm-out`).
//
// Per-frame layout / effect / source-state config flows over the
// per-instance gRPC control plane (Composer.SetCanvas / SetSource /
// SetLayout / SetEffects / SetSourceState). The boot argv is bare;
// everything dynamic is daemon-pushed post-spawn.
type ComposerStage struct {
	ComposerID string
	BinaryPath string
	DRMDevice  string
	CanvasFPS  int
	CanvasW    int
	CanvasH    int
	GrpcUds    string
}

// ComposerPoolKey returns the pool.Pool key for a composer.
func ComposerPoolKey(composerID string) string { return "composer:" + composerID }

// ComposerIDFor returns the stable composer-id identifier used by the
// daemon control plane.
func ComposerIDFor(composerID string) string { return composerID }

// ID returns the stage's process.Pool key.
func (c *ComposerStage) ID() string { return ComposerPoolKey(c.ComposerID) }

// Kind reports this as a Composer stage.
func (c *ComposerStage) Kind() Kind { return KindComposer }

// StreamID returns "" — composers are independent entities, not stream-
// scoped, in the post-refactor model.
func (c *ComposerStage) StreamID() string { return "" }

// SCMOutSocketPath returns the canvas output socket the composer binds.
func (c *ComposerStage) SCMOutSocketPath() string {
	return SCMSocketPathFor("composer-" + c.ComposerID)
}

// Command returns the videonode-composer argv.
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
		fps = DefaultCanvasFPS
	}
	argv := []string{
		c.BinaryPath,
		"--drm-device", c.DRMDevice,
		"--grpc-listen", c.GrpcUds,
		"--composer-id", c.ComposerID,
		"--scm-out", c.SCMOutSocketPath(),
		"--target-fps", strconv.Itoa(fps),
	}
	if c.CanvasW > 0 && c.CanvasH > 0 {
		argv = append(argv, "--canvas_w", strconv.Itoa(c.CanvasW))
		argv = append(argv, "--canvas_h", strconv.Itoa(c.CanvasH))
	}
	return argv, nil, nil
}

// LogParser uses the ffmpeg parser — composer emits the same
// `[level] msg` format via vn::log helpers.
func (c *ComposerStage) LogParser() process.LogParser { return ffmpeg.ParseLogLine }

// LogAttrs tags composer logs with the composer id + pool-key instance.
func (c *ComposerStage) LogAttrs() []slog.Attr {
	return []slog.Attr{
		slog.String(logging.KeyComposerID, c.ComposerID),
		slog.String(logging.KeyStageInstance, c.ID()),
	}
}
