package streams

import (
	"time"

	"github.com/smazurov/videonode/internal/ffmpeg"
	"github.com/smazurov/videonode/internal/streams/pipeline"
	"github.com/smazurov/videonode/internal/types"
)

// Source re-exports the canonical producer descriptor from the pipeline
// package. The service-layer split (B9) treats sources as first-class
// entities; this alias lets api/service consumers use one set of types.
type Source = pipeline.Source

// Composer re-exports the canonical composer descriptor.
type Composer = pipeline.Composer

// ComposerInput re-exports the canonical composer input descriptor.
type ComposerInput = pipeline.ComposerInput

// ComposerLayoutSlot re-exports the canonical composer layout slot.
type ComposerLayoutSlot = pipeline.LayoutSlot

// ComposerEffect re-exports the canonical composer effect descriptor.
type ComposerEffect = pipeline.Effect

// ComposerCanvasDims re-exports the canonical composer canvas dimensions.
type ComposerCanvasDims = pipeline.CanvasDims

// PipelineStream re-exports the canonical slim stream descriptor. (The
// short name "Stream" in this package already designates a runtime
// state struct; PipelineStream is the persisted shape used by Apply.)
type PipelineStream = pipeline.Stream

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

	AudioDevices []string `toml:"audio_devices,omitempty" json:"audio_devices,omitempty"` // one output audio track per entry

	// SourceOverrides parallels SourceStreams; non-empty length must match.
	SourceOverrides []CanvasSourceOverride `toml:"source_overrides,omitempty" json:"source_overrides,omitempty"`

	// LayoutName pins a candidate by name; unknown names are silently ignored and scorer runs.
	LayoutName string `toml:"layout_name,omitempty" json:"layout_name,omitempty"`

	// Enabled persists the engaged/dormant state across restarts. nil = engaged
	// (the default for a fresh canvas), false = released (dormant).
	Enabled *bool `toml:"enabled,omitempty" json:"enabled,omitempty"`
}

// IsEngaged reports whether the canvas should be running. A nil receiver or
// nil Enabled both mean engaged; only an explicit false marks the canvas dormant.
func (c *CanvasConfig) IsEngaged() bool {
	return c == nil || c.Enabled == nil || *c.Enabled
}

// CanvasSourceOverride shadows a source stream's settings for one canvas-item placement.
type CanvasSourceOverride struct {
	Rotation *int `toml:"rotation,omitempty" json:"rotation,omitempty"` // nil = inherit; 0, 90, 180, 270
}
