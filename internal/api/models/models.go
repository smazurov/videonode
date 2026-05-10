// Package models defines API request and response data structures.
package models

import (
	"fmt"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/smazurov/videonode/internal/ffmpeg"
)

// HealthData contains health check response fields.
type HealthData struct {
	Status  string `json:"status" example:"ok" doc:"Service status"`
	Message string `json:"message" example:"API is healthy" doc:"Status message"`
}

// HealthResponse wraps HealthData for API responses.
type HealthResponse struct {
	Body HealthData
}

// EncoderType represents the category of encoder (video or audio).
type EncoderType string

// Encoder type constants.
const (
	VideoEncoder EncoderType = "video"
	AudioEncoder EncoderType = "audio"
)

// EncoderData contains lists of available video and audio encoders.
type EncoderData struct {
	VideoEncoders []EncoderInfo `json:"video_encoders" doc:"Available video encoders"`
	AudioEncoders []EncoderInfo `json:"audio_encoders" doc:"Available audio encoders"`
	Count         int           `json:"count" example:"15" doc:"Total number of encoders"`
}

// EncoderInfo describes a single encoder with its capabilities.
type EncoderInfo struct {
	Type        EncoderType `json:"type" example:"video" doc:"Encoder type"`
	Name        string      `json:"name" example:"libx264" doc:"Encoder name"`
	Description string      `json:"description" example:"H.264 encoder" doc:"Human-readable description"`
	HWAccel     bool        `json:"hwaccel" example:"false" doc:"Whether this is a hardware-accelerated encoder"`
}

// EncodersResponse wraps EncoderData for API responses.
type EncodersResponse struct {
	Body EncoderData
}

// StreamData represents a video stream with its configuration and status.
type StreamData struct {
	StreamID  string    `json:"stream_id" example:"stream-001" doc:"Unique stream identifier"`
	DeviceID  string    `json:"device_id" example:"usb-0000:00:14.0-1" doc:"Stable device identifier"`
	Codec     string    `json:"codec" example:"h264" doc:"Video codec being used"`
	Bitrate   string    `json:"bitrate,omitempty" example:"2M" doc:"Video bitrate"`
	StartTime time.Time `json:"start_time,omitzero" doc:"When the stream was loaded into memory"`
	RTSPURL   string    `json:"rtsp_url,omitempty" example:"rtsp://localhost:8554/stream-001" doc:"RTSP streaming URL"`
	SRTURL    string    `json:"srt_url,omitempty" example:"srt://localhost:6001?streamid=stream-001" doc:"SRT streaming URL"`
	// Configuration fields for editing
	InputFormat     string   `json:"input_format,omitempty" example:"yuyv422" doc:"V4L2 input format"`
	Resolution      string   `json:"resolution,omitempty" example:"1920x1080" doc:"Video resolution"`
	Framerate       string   `json:"framerate,omitempty" example:"30" doc:"Video framerate"`
	Rotation        int      `json:"rotation,omitempty" enum:"0,90,180,270" example:"0" doc:"Output rotation in degrees (individual streams only)"`
	AudioDevice     string   `json:"audio_device,omitempty" example:"hw:4,0" doc:"ALSA audio device"`
	CustomFFmpegCmd string   `json:"custom_ffmpeg_command,omitempty" example:"ffmpeg -f v4l2..." doc:"Custom FFmpeg command override"`
	TestMode        bool     `json:"test_mode" example:"false" doc:"Test pattern mode enabled"`
	Enabled         bool     `json:"enabled" example:"true" doc:"Runtime state - device ready and stream active"`
	Options         []string `json:"options,omitempty" doc:"FFmpeg option keys (e.g., vsync_passthrough, low_latency)"`
	// Canvas fields (populated for canvas streams only)
	Canvas        *CanvasData       `json:"canvas,omitempty" doc:"Canvas configuration for composite streams"`
	Layout        *CanvasLayoutData `json:"layout,omitempty" doc:"Resolved canvas layout (slot + content rects). Populated for canvas streams."`
	InputsEnabled map[string]bool   `json:"inputs_enabled,omitempty" doc:"Per-source-stream-ID enabled state (canvas streams only)"`
	// OwnedBy is the canvas stream ID currently capturing this stream's device, if any.
	OwnedBy string `json:"owned_by,omitempty" doc:"Canvas ID currently owning this stream (individual streams only)"`
	// Vision and perspective
	Perspective *PerspectiveData `json:"perspective,omitempty" doc:"Perspective correction corners"`
	Vision      *VisionData      `json:"vision,omitempty" doc:"Vision pipeline config"`
}

// CanvasData represents a composite canvas configuration for API requests and responses.
type CanvasData struct {
	Width           int                        `json:"width" enum:"1920,3840" example:"1920" doc:"Canvas width — 1920 (1080p) or 3840 (4k)"`
	Height          int                        `json:"height" enum:"1080,2160" example:"1080" doc:"Canvas height — 1080 (1080p) or 2160 (4k)"`
	FPS             string                     `json:"fps" example:"30" doc:"Canvas output framerate"`
	KeyColor        string                     `json:"key_color,omitempty" example:"0x000000" doc:"Background color for dead space"`
	SourceStreams   []string                   `json:"source_streams" minItems:"1" maxItems:"4" doc:"Ordered list of source stream IDs (1–4)"`
	AudioDevices    []string                   `json:"audio_devices,omitempty" maxItems:"1" doc:"Standalone ALSA audio devices (v1: max 1)"`
	SourceOverrides []CanvasSourceOverrideData `json:"source_overrides,omitempty" doc:"Per-source-stream overrides. When set, length must equal source_streams. Each entry's nil fields inherit from the source stream."`
	LayoutName      string                     `json:"layout_name,omitempty" doc:"Pinned layout candidate name (e.g. \"side-by-side\", \"2x2\"). Empty = auto-pick."`
}

// CanvasSourceOverrideData shadows a source stream's settings for a single
// canvas-item placement. Nil fields inherit from the source.
type CanvasSourceOverrideData struct {
	Rotation *int `json:"rotation,omitempty" enum:"0,90,180,270" doc:"Rotation override in degrees; null to inherit from source stream"`
}

// CanvasLayoutData is the resolved slot + content geometry for a canvas.
type CanvasLayoutData struct {
	Slots            []CanvasLayoutSlotData `json:"slots" doc:"One entry per source stream, in the same order as canvas.source_streams"`
	ChosenLayout     string                 `json:"chosen_layout" doc:"Name of the layout candidate actually used"`
	AvailableLayouts []string               `json:"available_layouts" doc:"All candidate layout names available for the current source count, in default-first order"`
}

// CanvasLayoutSlotData describes one source's placement on the canvas.
// Coordinates are in canvas pixel space.
type CanvasLayoutSlotData struct {
	SourceStreamID       string  `json:"source_stream_id" doc:"Source stream ID for this slot"`
	SlotX                int     `json:"slot_x" doc:"Slot rectangle X — region allotted by the layout solver"`
	SlotY                int     `json:"slot_y" doc:"Slot rectangle Y"`
	SlotW                int     `json:"slot_w" doc:"Slot rectangle width"`
	SlotH                int     `json:"slot_h" doc:"Slot rectangle height"`
	ContentX             int     `json:"content_x" doc:"Content rectangle X — where the letterboxed input pixels actually land"`
	ContentY             int     `json:"content_y" doc:"Content rectangle Y"`
	ContentW             int     `json:"content_w" doc:"Content rectangle width"`
	ContentH             int     `json:"content_h" doc:"Content rectangle height"`
	EffectiveAspectRatio float64 `json:"effective_aspect_ratio" doc:"Input aspect ratio after perspective + rotation + crop; 0 when unknown"`
	RotationApplied      int     `json:"rotation_applied" enum:"0,90,180,270" doc:"Rotation the pipeline applies to this source (after override)"`
}

// CanvasLayoutRequest wraps CanvasData for the layout preview endpoint.
type CanvasLayoutRequest struct {
	Body CanvasData
}

// CanvasLayoutResponse wraps CanvasLayoutData for API responses.
type CanvasLayoutResponse struct {
	Body CanvasLayoutData
}

// StreamListData contains a list of all active streams.
type StreamListData struct {
	Streams []StreamData `json:"streams" doc:"List of active streams"`
	Count   int          `json:"count" example:"2" doc:"Number of active streams"`
}

// StreamListResponse wraps StreamListData for API responses.
type StreamListResponse struct {
	Body StreamListData
}

// CodecType represents a video codec standard.
type CodecType string

// Video codec constants.
const (
	CodecH264 CodecType = "h264"
	CodecH265 CodecType = "h265"
)

// StreamRequestData contains parameters for creating a new stream.
type StreamRequestData struct {
	StreamID    string      `json:"stream_id" pattern:"^[a-zA-Z0-9_-]+$" minLength:"1" maxLength:"50" example:"my-stream-001" doc:"Stream identifier"`
	DeviceID    string      `json:"device_id,omitempty" pattern:"^[^/]+" example:"usb-0000:00:14.0-1" doc:"Stable USB device identifier (required for single-camera streams)"`
	Codec       CodecType   `json:"codec" enum:"h264,h265" example:"h264" doc:"Video codec standard"`
	InputFormat string      `json:"input_format,omitempty" example:"yuyv422" doc:"V4L2 input format (required for single-camera streams)"`
	Bitrate     float64     `json:"bitrate,omitempty" example:"2.0" doc:"Bitrate in Mbps"`
	Width       int         `json:"width,omitempty" example:"1920" doc:"Video width"`
	Height      int         `json:"height,omitempty" example:"1080" doc:"Video height"`
	Framerate   int         `json:"framerate,omitempty" example:"30" doc:"Video framerate"`
	Rotation    int         `json:"rotation,omitempty" enum:"0,90,180,270" example:"0" doc:"Output rotation in degrees"`
	AudioDevice string      `json:"audio_device,omitempty" example:"hw:4,0" doc:"ALSA device for audio"`
	Options     []string    `json:"options,omitempty" doc:"FFmpeg option keys (e.g., vsync_passthrough, low_latency)"`
	Canvas      *CanvasData `json:"canvas,omitempty" doc:"Canvas configuration for composite streams referencing 1–4 individual streams"`
}

// Resolve validates conditional requirements based on stream type.
func (d *StreamRequestData) Resolve(_ huma.Context, prefix *huma.PathBuffer) []error {
	if d.Canvas == nil {
		var errs []error
		if d.DeviceID == "" {
			errs = append(errs, &huma.ErrorDetail{
				Location: prefix.With("device_id"),
				Message:  "required for single-camera streams",
				Value:    d.DeviceID,
			})
		}
		if d.InputFormat == "" {
			errs = append(errs, &huma.ErrorDetail{
				Location: prefix.With("input_format"),
				Message:  "required for single-camera streams",
				Value:    d.InputFormat,
			})
		}
		return errs
	}
	return d.Canvas.validate(prefix.With("canvas"))
}

// Resolve runs canvas-shape validation when a CanvasData is decoded as a
// request body — used by the layout preview endpoint.
func (c *CanvasData) Resolve(_ huma.Context, prefix *huma.PathBuffer) []error {
	return c.validate(prefix.String())
}

// validate is the single source of canvas-shape validation rules. It does not
// touch the stream store — caller must verify referenced source IDs exist.
func (c *CanvasData) validate(prefix string) []error {
	field := func(name string) string {
		if prefix == "" {
			return name
		}
		return prefix + "." + name
	}
	indexed := func(name string, i int) string {
		return fmt.Sprintf("%s[%d]", field(name), i)
	}
	var errs []error
	validSize := (c.Width == 1920 && c.Height == 1080) || (c.Width == 3840 && c.Height == 2160)
	if !validSize {
		errs = append(errs, &huma.ErrorDetail{
			Location: prefix,
			Message:  "canvas size must be 1920x1080 or 3840x2160",
			Value:    c,
		})
	}
	if c.FPS == "" {
		errs = append(errs, &huma.ErrorDetail{
			Location: field("fps"),
			Message:  "fps is required",
		})
	}
	if n := len(c.SourceStreams); n < 1 || n > 4 {
		errs = append(errs, &huma.ErrorDetail{
			Location: field("source_streams"),
			Message:  "canvas must reference 1–4 source streams",
			Value:    c.SourceStreams,
		})
	}
	seen := make(map[string]bool, len(c.SourceStreams))
	for i, srcID := range c.SourceStreams {
		if srcID == "" || seen[srcID] {
			errs = append(errs, &huma.ErrorDetail{
				Location: indexed("source_streams", i),
				Message:  "source stream IDs must be non-empty and unique",
				Value:    srcID,
			})
		}
		seen[srcID] = true
	}
	if len(c.SourceOverrides) > 0 && len(c.SourceOverrides) != len(c.SourceStreams) {
		errs = append(errs, &huma.ErrorDetail{
			Location: field("source_overrides"),
			Message:  "when set, source_overrides length must equal source_streams length",
			Value:    len(c.SourceOverrides),
		})
	}
	for i, ov := range c.SourceOverrides {
		if ov.Rotation != nil {
			switch *ov.Rotation {
			case 0, 90, 180, 270:
			default:
				errs = append(errs, &huma.ErrorDetail{
					Location: indexed("source_overrides", i) + ".rotation",
					Message:  "rotation must be 0, 90, 180, or 270",
					Value:    *ov.Rotation,
				})
			}
		}
	}
	if len(c.AudioDevices) > 1 {
		errs = append(errs, &huma.ErrorDetail{
			Location: field("audio_devices"),
			Message:  "canvas currently supports at most 1 audio device",
			Value:    c.AudioDevices,
		})
	}
	return errs
}

// StreamRequest wraps StreamRequestData for API requests.
type StreamRequest struct {
	Body StreamRequestData
}

// StreamUpdateRequestData contains optional fields for updating an existing stream.
type StreamUpdateRequestData struct {
	Codec               *string                   `json:"codec,omitempty" enum:"h264,h265" example:"h264" doc:"Video codec standard"`
	InputFormat         *string                   `json:"input_format,omitempty" example:"yuyv422" doc:"V4L2 input format"`
	Bitrate             *float64                  `json:"bitrate,omitempty" example:"2.0" doc:"Bitrate in Mbps"`
	Width               *int                      `json:"width,omitempty" example:"1920" doc:"Video width"`
	Height              *int                      `json:"height,omitempty" example:"1080" doc:"Video height"`
	Framerate           *int                      `json:"framerate,omitempty" example:"30" doc:"Video framerate"`
	Rotation            *int                      `json:"rotation,omitempty" enum:"0,90,180,270" example:"0" doc:"Output rotation in degrees"`
	AudioDevice         *string                   `json:"audio_device,omitempty" example:"hw:4,0" doc:"ALSA device for audio"`
	Options             []string                  `json:"options,omitempty" doc:"FFmpeg option keys (e.g., vsync_passthrough, low_latency)"`
	CustomFFmpegCommand *string                   `json:"custom_ffmpeg_command,omitempty" example:"ffmpeg -f v4l2..." doc:"Custom FFmpeg command override"`
	TestMode            *bool                     `json:"test_mode,omitempty" example:"false" doc:"Enable test pattern mode instead of device capture"`
	Enabled             *bool                     `json:"enabled,omitempty" example:"true" doc:"Manual override of runtime enabled state"`
	Canvas              *CanvasData               `json:"canvas,omitempty" doc:"Canvas configuration for composite streams (replaces entire canvas)"`
	Perspective         Nullable[PerspectiveData] `json:"perspective,omitzero" doc:"Perspective corners (null to clear)"`
	Vision              Nullable[VisionData]      `json:"vision,omitzero" doc:"Vision config (null to clear)"`
}

// StreamUpdateRequest wraps StreamUpdateRequestData for API requests.
type StreamUpdateRequest struct {
	Body StreamUpdateRequestData
}

// StreamResponse wraps StreamData for API responses.
type StreamResponse struct {
	Body StreamData
}

// StreamStatusData contains basic status information about a stream.
type StreamStatusData struct {
	StreamID  string    `json:"stream_id" example:"stream-001" doc:"Unique stream identifier"`
	StartTime time.Time `json:"start_time,omitzero" doc:"When the stream was loaded into memory"`
}

// StreamStatusResponse wraps StreamStatusData for API responses.
type StreamStatusResponse struct {
	Body StreamStatusData
}

// ErrorData contains error information for failed API requests.
type ErrorData struct {
	Status  string `json:"status" example:"error" doc:"Error status"`
	Message string `json:"message" example:"Device not found" doc:"Error message"`
}

// ErrorResponse wraps ErrorData for API responses.
type ErrorResponse struct {
	Body ErrorData
}

// OptionsData contains all available FFmpeg configuration options.
type OptionsData struct {
	Options []ffmpeg.Option `json:"options" doc:"All available FFmpeg options with metadata"`
}

// OptionsResponse wraps OptionsData for API responses.
type OptionsResponse struct {
	Body OptionsData
}

// VersionData contains build and version information about the application.
type VersionData struct {
	Version   string `json:"version" example:"dev" doc:"Application version"`
	GitCommit string `json:"git_commit" example:"abc1234" doc:"Git commit SHA"`
	BuildDate string `json:"build_date" example:"2024-12-15 14:30" doc:"Build timestamp"`
	BuildID   string `json:"build_id" example:"a1b2c3d4" doc:"Unique build identifier"`
	GoVersion string `json:"go_version" example:"go1.21.0" doc:"Go compiler version"`
	Compiler  string `json:"compiler" example:"gc" doc:"Compiler used"`
	Platform  string `json:"platform" example:"linux/amd64" doc:"Platform"`
}

// VersionResponse wraps VersionData for API responses.
type VersionResponse struct {
	Body VersionData
}

// FFmpegCommandData contains the FFmpeg command for a specific stream.
type FFmpegCommandData struct {
	StreamID string `json:"stream_id" example:"stream-001" doc:"Stream identifier"`
	Command  string `json:"command" example:"ffmpeg -f v4l2 -i /dev/video0 ..." doc:"Complete FFmpeg command"`
	IsCustom bool   `json:"is_custom" example:"false" doc:"Whether this is a custom command or auto-generated"`
}

// FFmpegCommandResponse wraps FFmpegCommandData for API responses.
type FFmpegCommandResponse struct {
	Body FFmpegCommandData
}

// PerspectiveData holds 4 corner points for perspective correction.
// Corners are clockwise: [0]=top-left, [1]=top-right, [2]=bottom-right, [3]=bottom-left.
type PerspectiveData struct {
	Corners [4][2]int `json:"corners" doc:"Four corner points [[x,y],...] clockwise: top-left, top-right, bottom-right, bottom-left"`
}

// VisionData configures the raw frame output for the AI vision sidecar.
type VisionData struct {
	Enabled bool `json:"enabled" doc:"Enable raw frame output for AI pipeline"`
	Width   int  `json:"width,omitempty" doc:"Raw frame width (default 640)"`
	Height  int  `json:"height,omitempty" doc:"Raw frame height (default 480)"`
	FPS     int  `json:"fps,omitempty" minimum:"0" maximum:"60" doc:"Vision pipe frame rate (1-60). 0 = use server default."`
}

// FFmpegCommandRequest contains a custom FFmpeg command to set for a stream.
type FFmpegCommandRequest struct {
	Body struct {
		Command string `json:"command" minLength:"1" example:"ffmpeg -f v4l2 -i /dev/video0 ..." doc:"Custom FFmpeg command to use"`
	}
}
