package pipeline

import "time"

// Source is a daemon-managed frame producer keyed by stable id. Each
// Source maps 1:1 to a `videonode-source` process; Composers and Streams
// reference it by id (`source:<id>`), never by device path. Sharing is
// expressed as multiple consumers naming the same Source — there is no
// device-level refcount in the new model.
//
// Exactly one of Device / TestMode must be set. TestMode swaps the V4L2
// capture path for the source binary's `--test-pattern` driver so the
// rest of the pipeline (composer + encoder + RTSP/SRT/WebRTC) can run on
// hardware-less dev/CI rigs without any real device attached.
type Source struct {
	ID     string `toml:"id" json:"id"`
	Device string `toml:"device,omitempty" json:"device,omitempty"`
	// TestMode swaps the V4L2 producer for the source binary's internal
	// test-pattern driver. Mutually exclusive with Device.
	TestMode bool `toml:"test_mode,omitempty" json:"test_mode,omitempty"`
	// Format is the operator-selected V4L2 capture format. When set, the
	// daemon pushes a SetFormat gRPC to the source after spawn (and on
	// every change) so the V4L2 device opens with the requested fourcc /
	// resolution / framerate. Nil = let the source binary auto-negotiate.
	Format    *SourceFormat `toml:"format,omitempty" json:"format,omitempty"`
	CreatedAt time.Time     `toml:"created_at" json:"created_at"`
	UpdatedAt time.Time     `toml:"updated_at" json:"updated_at"`
}

// SourceFormat is the V4L2 capture format pushed to videonode-source over
// gRPC SetFormat. FourCC is the 4-char V4L2 pixel-format string
// ("YUYV", "MJPG", "NV12", ...). FPS=0 lets the driver pick.
type SourceFormat struct {
	FourCC string `toml:"fourcc" json:"fourcc"`
	Width  uint32 `toml:"width" json:"width"`
	Height uint32 `toml:"height" json:"height"`
	FPS    uint32 `toml:"fps,omitempty" json:"fps,omitempty"`
}

// SourceIDFor returns the canonical `source:<id>` reference used by
// Composer inputs and Stream.Upstream.
func SourceIDFor(id string) string { return "source:" + id }

// SourcePoolKey returns the process.Pool key for a Source. Matches the
// legacy `producer:<id>` convention so process-manager UI / metrics
// labels stay stable across the refactor.
func SourcePoolKey(id string) string { return "producer:" + id }
