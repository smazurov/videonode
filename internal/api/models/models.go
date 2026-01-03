// Package models defines API request and response data structures.
package models

import (
	"time"

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

// CaptureRequestData contains parameters for capturing a screenshot from a video device.
type CaptureRequestData struct {
	DevicePath string  `json:"devicePath" example:"/dev/video0" doc:"Path to the video device"`
	Resolution string  `json:"resolution,omitempty" example:"1920x1080" doc:"Optional resolution in format widthxheight"`
	Delay      float64 `json:"delay,omitempty" example:"2.0" doc:"Optional delay in seconds before capturing"`
}

// CaptureRequest wraps CaptureRequestData for API requests.
type CaptureRequest struct {
	Body CaptureRequestData
}

// CaptureData contains the result of a capture operation.
type CaptureData struct {
	Status  string            `json:"status" example:"success" doc:"Capture status"`
	Message string            `json:"message" example:"Screenshot captured successfully" doc:"Status message"`
	Data    map[string]string `json:"data,omitempty" doc:"Additional data (base64 image for API requests)"`
}

// CaptureResponse wraps CaptureData for API responses.
type CaptureResponse struct {
	Body CaptureData
}

// StreamData represents a video stream with its configuration and status.
type StreamData struct {
	StreamID  string    `json:"stream_id" example:"stream-001" doc:"Unique stream identifier"`
	DeviceID  string    `json:"device_id" example:"usb-0000:00:14.0-1" doc:"Stable device identifier"`
	Codec     string    `json:"codec" example:"h264" doc:"Video codec being used"`
	Bitrate   string    `json:"bitrate,omitempty" example:"2M" doc:"Video bitrate"`
	StartTime time.Time `json:"start_time,omitzero" doc:"When the stream was loaded into memory"`
	WebRTCURL string    `json:"webrtc_url,omitempty" example:"webrtc://localhost:8090/stream-001" doc:"WebRTC streaming URL"`
	SRTURL    string    `json:"srt_url,omitempty" example:"srt://localhost:8890?streamid=read:stream-001" doc:"SRT URL"`
	// Configuration fields for editing
	InputFormat     string   `json:"input_format,omitempty" example:"yuyv422" doc:"V4L2 input format"`
	Resolution      string   `json:"resolution,omitempty" example:"1920x1080" doc:"Video resolution"`
	Framerate       string   `json:"framerate,omitempty" example:"30" doc:"Video framerate"`
	AudioDevice     string   `json:"audio_device,omitempty" example:"hw:4,0" doc:"ALSA audio device"`
	CustomFFmpegCmd string   `json:"custom_ffmpeg_command,omitempty" example:"ffmpeg -f v4l2..." doc:"Custom FFmpeg command override"`
	TestMode        bool     `json:"test_mode" example:"false" doc:"Test pattern mode enabled"`
	Enabled         bool     `json:"enabled" example:"true" doc:"Runtime state - device ready and stream active"`
	Options         []string `json:"options,omitempty" doc:"FFmpeg option keys (e.g., vsync_passthrough, low_latency)"`
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
	StreamID    string    `json:"stream_id" pattern:"^[a-zA-Z0-9_-]+$" minLength:"1" maxLength:"50" example:"my-stream-001" doc:"Stream identifier"`
	DeviceID    string    `json:"device_id" minLength:"1" pattern:"^[^/]+" example:"usb-0000:00:14.0-1" doc:"Stable USB device identifier"`
	Codec       CodecType `json:"codec" enum:"h264,h265" example:"h264" doc:"Video codec standard"`
	InputFormat string    `json:"input_format" minLength:"1" example:"yuyv422" doc:"V4L2 input format"`
	Bitrate     float64   `json:"bitrate,omitempty" example:"2.0" doc:"Bitrate in Mbps"`
	Width       int       `json:"width,omitempty" example:"1920" doc:"Video width"`
	Height      int       `json:"height,omitempty" example:"1080" doc:"Video height"`
	Framerate   int       `json:"framerate,omitempty" example:"30" doc:"Video framerate"`
	AudioDevice string    `json:"audio_device,omitempty" example:"hw:4,0" doc:"ALSA device for audio"`
	Options     []string  `json:"options,omitempty" doc:"FFmpeg option keys (e.g., vsync_passthrough, low_latency)"`
}

// StreamRequest wraps StreamRequestData for API requests.
type StreamRequest struct {
	Body StreamRequestData
}

// StreamUpdateRequestData contains optional fields for updating an existing stream.
type StreamUpdateRequestData struct {
	Codec               *string  `json:"codec,omitempty" enum:"h264,h265" example:"h264" doc:"Video codec standard"`
	InputFormat         *string  `json:"input_format,omitempty" example:"yuyv422" doc:"V4L2 input format"`
	Bitrate             *float64 `json:"bitrate,omitempty" example:"2.0" doc:"Bitrate in Mbps"`
	Width               *int     `json:"width,omitempty" example:"1920" doc:"Video width"`
	Height              *int     `json:"height,omitempty" example:"1080" doc:"Video height"`
	Framerate           *int     `json:"framerate,omitempty" example:"30" doc:"Video framerate"`
	AudioDevice         *string  `json:"audio_device,omitempty" example:"hw:4,0" doc:"ALSA device for audio"`
	Options             []string `json:"options,omitempty" doc:"FFmpeg option keys (e.g., vsync_passthrough, low_latency)"`
	CustomFFmpegCommand *string  `json:"custom_ffmpeg_command,omitempty" example:"ffmpeg -f v4l2..." doc:"Custom FFmpeg command override"`
	TestMode            *bool    `json:"test_mode,omitempty" example:"false" doc:"Enable test pattern mode instead of device capture"`
	Enabled             *bool    `json:"enabled,omitempty" example:"true" doc:"Manual override of runtime enabled state"`
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

// FFmpegCommandRequest contains a custom FFmpeg command to set for a stream.
type FFmpegCommandRequest struct {
	Body struct {
		Command string `json:"command" minLength:"1" example:"ffmpeg -f v4l2 -i /dev/video0 ..." doc:"Custom FFmpeg command to use"`
	}
}
