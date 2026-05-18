package ffmpeg

// Params holds all parameters needed to generate an FFmpeg command.
type Params struct {
	DevicePath  string
	InputFormat string
	Resolution  string
	FPS         string
	OverlayText string // non-empty forces lavfi testsrc2

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

	AudioDevice  string
	AudioFilters string

	ProgressSocket string
	OutputURL      string

	Options []OptionType

	VisionEnabled bool
	VisionWidth   int // default 640
	VisionHeight  int // default 480
	VisionFPS     int // 0 = no throttle

	Perspective *PerspectiveConfig

	Rotation int // 0, 90, 180, 270

	HWCaps HWCapabilities
}

// PerspectiveConfig stores 4 source corner points clockwise [TL, TR, BR, BL] in input pixels.
type PerspectiveConfig struct {
	Corners [4][2]int `toml:"corners" json:"corners"`
}

// VisionConfig enables raw frame output for the AI vision sidecar.
type VisionConfig struct {
	Enabled bool `toml:"enabled" json:"enabled"`
	Width   int  `toml:"width,omitempty" json:"width,omitempty"`
	Height  int  `toml:"height,omitempty" json:"height,omitempty"`
	FPS     int  `toml:"fps,omitempty" json:"fps,omitempty"`
}
