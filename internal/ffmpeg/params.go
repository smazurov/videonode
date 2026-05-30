package ffmpeg

// Params holds all parameters needed to generate an FFmpeg command.
type Params struct {
	DevicePath  string
	InputFormat string
	Resolution  string
	FPS         string
	OverlayText string // non-empty forces lavfi testsrc2

	// InputPipe, when non-nil, replaces the `-f v4l2 -i <DevicePath>`
	// block with a stdin pipe input. Used by the native pipeline path
	// where vn-sink or videonode-composer pipes frames into ffmpeg.
	InputPipe *PipeInput

	Encoder string

	Bitrate    string
	MinRate    string
	MaxRate    string
	BufferSize string
	CRF        int // 0 = not set
	QP         int // 0 = not set
	RCMode     string

	Preset  string
	GOP     int // 0 = not set
	BFrames int // -1 = not set

	GlobalArgs   []string
	VideoFilters string
	HWBackend    string // "rkmpp", "vaapi", "sw", or ""

	AudioDevice string

	// AudioCodec is the logical output codec ("opus", "aac"); empty defaults
	// to opus. AudioBitrate is the output audio bitrate (e.g. "128k"); empty
	// defaults to 128k.
	AudioCodec   string
	AudioBitrate string

	// AudioFilters, for multi-input audio, is a mix filtergraph (e.g.
	// "amix=inputs=2:duration=shortest") applied after per-input aresample to
	// produce a single mixed output track; for single-input audio it is an
	// -af chain. Empty keeps the default one-track-per-input behavior.
	AudioFilters string

	// AudioInputs, when non-empty, supersedes AudioDevice and declares
	// one ALSA input per entry. With no AudioFilters the builder emits an
	// aresample-per-input filter_complex with one output track per input;
	// with AudioFilters set it mixes them into a single track.
	AudioInputs []string

	ProgressSocket string
	OutputURL      string

	// Outputs, when non-empty, supersedes OutputURL and emits one
	// `-f <muxer> <url>` per entry. Muxer is selected from the type
	// string ("rtsp", "srt", "hls"); unknown types pass through verbatim.
	Outputs []OutputTarget

	Options []OptionType

	VisionEnabled bool
	VisionWidth   int // default 640
	VisionHeight  int // default 480
	VisionFPS     int // 0 = no throttle

	Perspective *PerspectiveConfig

	Rotation int // 0, 90, 180, 270

	HWCaps HWCapabilities
}

// PipeInput describes a stdin frame stream piped into ffmpeg from an
// upstream process (vn-sink, videonode-composer). Format selects the
// ffmpeg `-f` muxer; rawvideo also needs PixelFormat + dims + framerate
// at the input stage (it is not self-describing).
type PipeInput struct {
	Format      string // "yuv4mpegpipe" or "rawvideo"
	PixelFormat string // e.g. "bgra"; only used for rawvideo
	Width       int    // only used for rawvideo
	Height      int    // only used for rawvideo
	FPS         int    // only used for rawvideo
}

// OutputTarget is one publish destination. Type names a transport
// ("rtsp", "srt", "hls"); URL is the full ffmpeg output URL.
type OutputTarget struct {
	Type string
	URL  string
}

// PerspectiveConfig stores 4 source corner points clockwise
// [TL, TR, BR, BL] in input pixels, together with the dimensions of the
// snapshot frame the user clicked on to mark them.
//
// SnapshotWidth/SnapshotHeight are used by the GPU composer path to
// normalize corner pixel coordinates into [0,1] UV space; the resulting
// homography is dimension-agnostic, so subsequent source-resolution
// changes (e.g. HDMI mode switch) don't invalidate the warp.
//
// The ffmpeg CPU path consumes corners directly in input-pixel space
// and ignores SnapshotWidth/Height — they're metadata for the GPU side.
// When older streams.toml entries lack these fields, callers fall back
// to the source's currently-configured input dimensions (logged once).
type PerspectiveConfig struct {
	Corners        [4][2]int `toml:"corners" json:"corners"`
	SnapshotWidth  int       `toml:"snapshot_width,omitempty" json:"snapshot_width,omitempty"`
	SnapshotHeight int       `toml:"snapshot_height,omitempty" json:"snapshot_height,omitempty"`
}

// VisionConfig enables raw frame output for the AI vision sidecar.
type VisionConfig struct {
	Enabled bool `toml:"enabled" json:"enabled"`
	Width   int  `toml:"width,omitempty" json:"width,omitempty"`
	Height  int  `toml:"height,omitempty" json:"height,omitempty"`
	FPS     int  `toml:"fps,omitempty" json:"fps,omitempty"`
}
