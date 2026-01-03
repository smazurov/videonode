package streams

import (
	"time"

	"github.com/smazurov/videonode/internal/ffmpeg"
	"github.com/smazurov/videonode/internal/types"
)

// StreamSpec represents a single stream specification.
// This is the persistent configuration for each stream, containing metadata
// and nested FFmpeg configuration.
type StreamSpec struct {
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

// FFmpegConfig contains FFmpeg settings embedded in StreamSpec.
// This is used for TOML marshaling and internal stream configuration.
type FFmpegConfig struct {
	// Codec specifies the desired codec standard (NOT the encoder implementation)
	// Values: "h264", "h265"
	Codec string `toml:"codec,omitempty" json:"codec,omitempty"`

	// InputFormat specifies the V4L2 pixel format to request from the device
	InputFormat string `toml:"input_format,omitempty" json:"input_format,omitempty"`

	// Resolution specifies the video dimensions in WIDTHxHEIGHT format
	Resolution string `toml:"resolution,omitempty" json:"resolution,omitempty"`

	// FPS specifies the framerate to capture from the device
	FPS string `toml:"fps,omitempty" json:"fps,omitempty"`

	// AudioDevice specifies the ALSA device for audio capture
	AudioDevice string `toml:"audio_device,omitempty" json:"audio_device,omitempty"`

	// Options contains FFmpeg behavior flags
	Options []ffmpeg.OptionType `toml:"options,omitempty" json:"options,omitempty"`

	// QualityParams stores the quality/rate control settings
	QualityParams *types.QualityParams `toml:"quality_params,omitempty" json:"quality_params,omitempty"`
}
