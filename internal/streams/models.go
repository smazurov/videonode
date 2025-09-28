package streams

import (
	"time"

	"github.com/smazurov/videonode/internal/ffmpeg"
	"github.com/smazurov/videonode/internal/types"
)

// FFmpegConfig contains user-specified FFmpeg settings for a stream.
// This structure holds only the user's intent, not hardware-specific implementations.
// Encoder selection and hardware-specific settings are injected at runtime.
type FFmpegConfig struct {
	// Codec specifies the desired codec standard (NOT the encoder implementation)
	// Values: "h264", "h265"
	// The actual encoder (h264_vaapi, libx264, etc.) is selected at runtime
	Codec string `toml:"codec,omitempty" json:"codec,omitempty"`

	// InputFormat specifies the V4L2 pixel format to request from the device
	// Common values: "yuyv422" (uncompressed), "mjpeg" (compressed)
	InputFormat string `toml:"input_format,omitempty" json:"input_format,omitempty"`

	// Resolution specifies the video dimensions in WIDTHxHEIGHT format
	// Example: "1920x1080", "1280x720"
	Resolution string `toml:"resolution,omitempty" json:"resolution,omitempty"`

	// FPS specifies the framerate to capture from the device
	// Example: "30", "60", "29.97"
	FPS string `toml:"fps,omitempty" json:"fps,omitempty"`

	// Options contains FFmpeg behavior flags that affect input/output handling
	// These are predefined options from ffmpeg.OptionType
	// Examples: "thread_queue_1024" (buffer size), "copyts" (timestamp handling)
	Options []ffmpeg.OptionType `toml:"options,omitempty" json:"options,omitempty"`

	// QualityParams stores the quality/rate control settings
	// This is used to generate encoder-specific parameters at runtime
	QualityParams *types.QualityParams `toml:"quality_params,omitempty" json:"quality_params,omitempty"`

	// AudioDevice specifies the ALSA device for audio capture
	// If set, enables audio passthrough
	// Example: "hw:4,0" for card 4, device 0
	AudioDevice string `toml:"audio_device,omitempty" json:"audio_device,omitempty"`
}

// StreamConfig represents a single stream configuration in the TOML file.
// This is the top-level structure for each stream, containing metadata
// and the nested FFmpeg configuration.
type StreamConfig struct {
	// ID is the unique identifier for this stream
	// This becomes the MediaMTX path name and must be unique
	ID string `toml:"id" json:"id"`

	// Name is a human-readable name for the stream
	// If not specified, defaults to the ID
	Name string `toml:"name" json:"name"`

	// Device is the stable USB device identifier
	// Format: "usb-BUS-PORT" (e.g., "usb-0000:00:14.0-1.2")
	// This is resolved to a /dev/videoX path at runtime
	Device string `toml:"device" json:"device"`

	// Enabled determines if this stream should be active
	// Disabled streams are kept in config but not started
	Enabled bool `toml:"enabled" json:"enabled"`

	// TestMode determines if stream should use test pattern instead of device
	// When true, generates test video/audio instead of capturing from device
	TestMode bool `toml:"test_mode" json:"test_mode"`

	// FFmpeg contains all FFmpeg-specific configuration for this stream
	FFmpeg FFmpegConfig `toml:"ffmpeg" json:"ffmpeg"`

	// CustomFFmpegCommand is an optional override for the entire FFmpeg command
	// When set, this completely bypasses automatic command generation
	CustomFFmpegCommand string `toml:"custom_ffmpeg_command,omitempty" json:"custom_ffmpeg_command,omitempty"`

	// CreatedAt timestamp when the stream was first created
	CreatedAt time.Time `toml:"created_at" json:"created_at"`

	// UpdatedAt timestamp when the stream was last modified
	UpdatedAt time.Time `toml:"updated_at" json:"updated_at"`
}

// StreamsConfig represents the complete streams configuration file
type StreamsConfig struct {
	Version    int                      `toml:"version" json:"version"`
	Validation *types.ValidationResults `toml:"validation,omitempty" json:"validation,omitempty"`
	Streams    map[string]StreamConfig  `toml:"streams" json:"streams"`
}
