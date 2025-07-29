package models

import (
	"time"
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

type StreamRequestData struct {
	StreamID  string `json:"stream_id" pattern:"^[a-zA-Z0-9_-]+$" minLength:"1" maxLength:"50" example:"my-stream-001" doc:"User-defined stream identifier (alphanumeric, dashes, underscores only)"`
	DeviceID  string `json:"device_id" minLength:"1" example:"usb-0000:00:14.0-1" doc:"Stable device identifier"`
	Codec     string `json:"codec" minLength:"1" example:"h264" doc:"Video codec to use"`
	Bitrate   int    `json:"bitrate,omitempty" example:"2000" doc:"Bitrate in kbps"`
	Width     int    `json:"width,omitempty" example:"1920" doc:"Video width"`
	Height    int    `json:"height,omitempty" example:"1080" doc:"Video height"`
	Framerate int    `json:"framerate,omitempty" example:"30" doc:"Video framerate"`
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
