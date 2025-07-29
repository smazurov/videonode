package server

import (
	"time"

	"github.com/smazurov/videonode/internal/config"
	"github.com/smazurov/videonode/internal/ffmpeg"
	"github.com/smazurov/videonode/v4l2_detector"
)

// ApiResponse represents a generic API response
type ApiResponse struct {
	Status  string      `json:"status"`
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
}

// DeviceResponse represents the response for device listing
type DeviceResponse struct {
	Devices []v4l2_detector.DeviceInfo `json:"devices"`
	Count   int                        `json:"count"`
}

// DeviceCapabilitiesResponse represents the capabilities of a specific device
type DeviceCapabilitiesResponse struct {
	DevicePath  string                     `json:"device_path"`
	Formats     []v4l2_detector.FormatInfo `json:"formats"`
	Resolutions []v4l2_detector.Resolution `json:"resolutions,omitempty"`
	Framerates  []v4l2_detector.Framerate  `json:"framerates,omitempty"`
}

// EncoderResponse represents the response for encoder listing
type EncoderResponse struct {
	Encoders EncoderList `json:"encoders"`
	Count    int         `json:"count"`
}

// CaptureRequest represents a request to capture a screenshot
type CaptureRequest struct {
	DevicePath string  `json:"devicePath"`
	Resolution string  `json:"resolution,omitempty"` // Optional resolution in format "widthxheight"
	Delay      float64 `json:"delay,omitempty"`      // Optional delay in seconds before capturing (for "no signal" messages)
}

// StreamRequest represents a request to start a stream
type StreamRequest struct {
	DevicePath string `json:"device_path"`
	Codec      string `json:"codec"`
	Bitrate    int    `json:"bitrate,omitempty"` // in kbps
	Width      int    `json:"width,omitempty"`
	Height     int    `json:"height,omitempty"`
	Framerate  int    `json:"framerate,omitempty"`
}

// StreamResponse represents a response for stream operations
type StreamResponse struct {
	StreamID   string        `json:"stream_id"`
	DevicePath string        `json:"device_path"`
	Codec      string        `json:"codec"`
	IsRunning  bool          `json:"is_running"`
	Uptime     time.Duration `json:"uptime,omitempty"`
	StartTime  time.Time     `json:"start_time,omitempty"`
	WebRTCURL  string        `json:"webrtc_url,omitempty"`
	RTSPURL    string        `json:"rtsp_url,omitempty"`
}

// StreamListResponse represents a list of active streams
type StreamListResponse struct {
	Streams []StreamResponse `json:"streams"`
	Count   int              `json:"count"`
}

// StreamConfigRequest represents a request to create/update a stream configuration
type StreamConfigRequest struct {
	ID            string              `json:"id"`
	Name          string              `json:"name"`
	Device        string              `json:"device"`
	Resolution    string              `json:"resolution,omitempty"`
	FPS           string              `json:"fps,omitempty"`
	Codec         string              `json:"codec,omitempty"`
	Preset        string              `json:"preset,omitempty"`
	Bitrate       string              `json:"bitrate,omitempty"`
	Enabled       bool                `json:"enabled"`
	FFmpegOptions []ffmpeg.OptionType `json:"ffmpeg_options,omitempty"` // Strongly typed FFmpeg options
}

// StreamConfigResponse represents a response for stream configuration operations
type StreamConfigResponse struct {
	StreamConfig config.StreamConfig `json:"stream_config"`
}

// StreamConfigListResponse represents a list of stream configurations
type StreamConfigListResponse struct {
	Configs []config.StreamConfig `json:"configs"`
	Count   int                   `json:"count"`
}

