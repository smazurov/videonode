package models

import (
	"time"

	"github.com/smazurov/videonode/internal/ffmpeg"
)

// Health check models
type HealthData struct {
	Status  string `json:"status" example:"ok" doc:"Service status"`
	Message string `json:"message" example:"API is healthy" doc:"Status message"`
}

type HealthResponse struct {
	Body HealthData
}

// Encoder models
type EncoderType string

const (
	VideoEncoder EncoderType = "video"
	AudioEncoder EncoderType = "audio"
)

type EncoderData struct {
	VideoEncoders []EncoderInfo `json:"video_encoders" doc:"Available video encoders"`
	AudioEncoders []EncoderInfo `json:"audio_encoders" doc:"Available audio encoders"`
	Count         int           `json:"count" example:"15" doc:"Total number of encoders"`
}

type EncoderInfo struct {
	Type        EncoderType `json:"type" example:"video" doc:"Encoder type"`
	Name        string      `json:"name" example:"libx264" doc:"Encoder name"`
	Description string      `json:"description" example:"H.264 encoder" doc:"Human-readable description"`
	HWAccel     bool        `json:"hwaccel" example:"false" doc:"Whether this is a hardware-accelerated encoder"`
}

type EncodersResponse struct {
	Body EncoderData
}

// Capture models
type CaptureRequestData struct {
	DevicePath string  `json:"devicePath" example:"/dev/video0" doc:"Path to the video device"`
	Resolution string  `json:"resolution,omitempty" example:"1920x1080" doc:"Optional resolution in format widthxheight"`
	Delay      float64 `json:"delay,omitempty" example:"2.0" doc:"Optional delay in seconds before capturing"`
}

type CaptureRequest struct {
	Body CaptureRequestData
}

type CaptureData struct {
	Status  string            `json:"status" example:"success" doc:"Capture status"`
	Message string            `json:"message" example:"Screenshot captured successfully" doc:"Status message"`
	Data    map[string]string `json:"data,omitempty" doc:"Additional data (base64 image for API requests)"`
}

type CaptureResponse struct {
	Body CaptureData
}

// Stream models
type StreamData struct {
	StreamID  string        `json:"stream_id" example:"stream-001" doc:"Unique stream identifier"`
	DeviceID  string        `json:"device_id" example:"usb-0000:00:14.0-1" doc:"Stable device identifier"`
	Codec     string        `json:"codec" example:"h264" doc:"Video codec being used"`
	Bitrate   string        `json:"bitrate,omitempty" example:"2M" doc:"Video bitrate (e.g., 2M, 1500k)"`
	Uptime    time.Duration `json:"uptime,omitempty" example:"3600000000000" doc:"Stream uptime in nanoseconds"`
	StartTime time.Time     `json:"start_time,omitempty" doc:"When the stream was started"`
	WebRTCURL string        `json:"webrtc_url,omitempty" example:"webrtc://localhost:8090/stream-001" doc:"WebRTC streaming URL"`
	RTSPURL   string        `json:"rtsp_url,omitempty" example:"rtsp://localhost:8554/stream-001" doc:"RTSP streaming URL"`
}

type StreamListData struct {
	Streams []StreamData `json:"streams" doc:"List of active streams"`
	Count   int          `json:"count" example:"2" doc:"Number of active streams"`
}

type StreamListResponse struct {
	Body StreamListData
}

type CodecType string

const (
	CodecH264 CodecType = "h264"
	CodecH265 CodecType = "h265"
)

type StreamRequestData struct {
	StreamID    string    `json:"stream_id" pattern:"^[a-zA-Z0-9_-]+$" minLength:"1" maxLength:"50" example:"my-stream-001" doc:"User-defined stream identifier (alphanumeric, dashes, underscores only)"`
	DeviceID    string    `json:"device_id" minLength:"1" pattern:"^[^/]+" example:"usb-0000:00:14.0-1" doc:"Stable USB device identifier (cannot start with /)"`
	Codec       CodecType `json:"codec" enum:"h264,h265" example:"h264" doc:"Video codec standard"`
	InputFormat string    `json:"input_format" minLength:"1" example:"yuyv422" doc:"V4L2 input format"`
	Bitrate     float64   `json:"bitrate,omitempty" example:"2.0" doc:"Bitrate in Mbps"`
	Width       int       `json:"width,omitempty" example:"1920" doc:"Video width"`
	Height      int       `json:"height,omitempty" example:"1080" doc:"Video height"`
	Framerate   int       `json:"framerate,omitempty" example:"30" doc:"Video framerate"`
	AudioDevice string    `json:"audio_device,omitempty" example:"hw:4,0" doc:"ALSA audio device (if set, enables audio passthrough)"`
	Options     []string  `json:"options,omitempty" doc:"FFmpeg option keys (e.g., vsync_passthrough, low_latency)"`
}

type StreamRequest struct {
	Body StreamRequestData
}

type StreamResponse struct {
	Body StreamData
}

// Stream status models
type StreamStatusData struct {
	StreamID  string        `json:"stream_id" example:"stream-001" doc:"Unique stream identifier"`
	Uptime    time.Duration `json:"uptime,omitempty" example:"3600000000000" doc:"Stream uptime in nanoseconds"`
	StartTime time.Time     `json:"start_time,omitempty" doc:"When the stream was started"`
}

type StreamStatusResponse struct {
	Body StreamStatusData
}

// Error response
type ErrorData struct {
	Status  string `json:"status" example:"error" doc:"Error status"`
	Message string `json:"message" example:"Device not found" doc:"Error message"`
}

type ErrorResponse struct {
	Body ErrorData
}

// Options models for FFmpeg configuration
type OptionsData struct {
	Options []ffmpeg.Option `json:"options" doc:"All available FFmpeg options with metadata"`
}

type OptionsResponse struct {
	Body OptionsData
}

// Version models
type VersionData struct {
	Version   string `json:"version" example:"dev" doc:"Application version"`
	GitCommit string `json:"git_commit" example:"abc1234" doc:"Git commit SHA"`
	BuildDate string `json:"build_date" example:"2024-12-15 14:30" doc:"Build timestamp"`
	BuildID   string `json:"build_id" example:"a1b2c3d4" doc:"Unique build identifier"`
	GoVersion string `json:"go_version" example:"go1.21.0" doc:"Go compiler version"`
	Compiler  string `json:"compiler" example:"gc" doc:"Compiler used"`
	Platform  string `json:"platform" example:"linux/amd64" doc:"Platform"`
}

type VersionResponse struct {
	Body VersionData
}

// FFmpeg command models
type FFmpegCommandData struct {
	StreamID string `json:"stream_id" example:"stream-001" doc:"Stream identifier"`
	Command  string `json:"command" example:"ffmpeg -f v4l2 -i /dev/video0 ..." doc:"Complete FFmpeg command"`
	IsCustom bool   `json:"is_custom" example:"false" doc:"Whether this is a custom command or auto-generated"`
}

type FFmpegCommandResponse struct {
	Body FFmpegCommandData
}

type FFmpegCommandRequest struct {
	Body struct {
		Command string `json:"command" minLength:"1" example:"ffmpeg -f v4l2 -i /dev/video0 ..." doc:"Custom FFmpeg command to use"`
	}
}

// ReloadResponse is the response for stream reload operation
type ReloadResponse struct {
	Body struct {
		Message string `json:"message" doc:"Operation result message"`
		Count   int    `json:"count" doc:"Number of streams synced"`
	}
}
