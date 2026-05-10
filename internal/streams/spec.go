package streams

import (
	"time"

	"github.com/smazurov/videonode/internal/ffmpeg"
	"github.com/smazurov/videonode/internal/types"
)

// StreamSpec is the persistent configuration for one stream.
type StreamSpec struct {
	ID       string `toml:"id" json:"id"`
	Name     string `toml:"name" json:"name"`
	Device   string `toml:"device" json:"device"` // "usb-BUS-PORT", resolved to /dev/videoX at runtime
	TestMode bool   `toml:"test_mode" json:"test_mode"`

	FFmpeg FFmpegConfig `toml:"ffmpeg" json:"ffmpeg"`

	// Canvas, when set, makes this a composite stream and overrides the single-camera fields above.
	Canvas *CanvasConfig `toml:"canvas,omitempty" json:"canvas,omitempty"`

	CustomFFmpegCommand string `toml:"custom_ffmpeg_command,omitempty" json:"custom_ffmpeg_command,omitempty"`

	Perspective *ffmpeg.PerspectiveConfig `toml:"perspective,omitempty" json:"perspective,omitempty"`
	Vision      *ffmpeg.VisionConfig      `toml:"vision,omitempty" json:"vision,omitempty"`

	CreatedAt time.Time `toml:"created_at" json:"created_at"`
	UpdatedAt time.Time `toml:"updated_at" json:"updated_at"`
}

// FFmpegConfig contains FFmpeg settings embedded in StreamSpec.
type FFmpegConfig struct {
	Codec         string               `toml:"codec,omitempty" json:"codec,omitempty"` // "h264" or "h265" (not the encoder name)
	InputFormat   string               `toml:"input_format,omitempty" json:"input_format,omitempty"`
	Resolution    string               `toml:"resolution,omitempty" json:"resolution,omitempty"`
	FPS           string               `toml:"fps,omitempty" json:"fps,omitempty"`
	AudioDevice   string               `toml:"audio_device,omitempty" json:"audio_device,omitempty"`
	Options       []ffmpeg.OptionType  `toml:"options,omitempty" json:"options,omitempty"`
	QualityParams *types.QualityParams `toml:"quality_params,omitempty" json:"quality_params,omitempty"`
	Rotation      int                  `toml:"rotation,omitempty" json:"rotation,omitempty"` // 0, 90, 180, 270
}

// CanvasConfig defines a composite canvas live-referencing 1–4 other streams.
type CanvasConfig struct {
	Width    int    `toml:"width" json:"width"`   // 1920 or 3840
	Height   int    `toml:"height" json:"height"` // 1080 or 2160
	FPS      string `toml:"fps" json:"fps"`
	KeyColor string `toml:"key_color,omitempty" json:"key_color,omitempty"`

	SourceStreams []string `toml:"source_streams" json:"source_streams"` // ordered, 1–4

	AudioDevices []string `toml:"audio_devices,omitempty" json:"audio_devices,omitempty"` // v1 uses at most one

	// SourceOverrides parallels SourceStreams; non-empty length must match.
	SourceOverrides []CanvasSourceOverride `toml:"source_overrides,omitempty" json:"source_overrides,omitempty"`

	// LayoutName pins a candidate by name; unknown names are silently ignored and scorer runs.
	LayoutName string `toml:"layout_name,omitempty" json:"layout_name,omitempty"`
}

// CanvasSourceOverride shadows a source stream's settings for one canvas-item placement.
type CanvasSourceOverride struct {
	Rotation *int `toml:"rotation,omitempty" json:"rotation,omitempty"` // nil = inherit; 0, 90, 180, 270
}
