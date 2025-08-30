package streams

import "time"

// Stream represents a video stream
type Stream struct {
	ID             string    `json:"stream_id"`
	DeviceID       string    `json:"device_id"`
	Codec          string    `json:"codec"`
	Bitrate        string    `json:"bitrate"` // Bitrate string from config (e.g., "2M", "1500k")
	StartTime      time.Time `json:"start_time"`
	WebRTCURL      string    `json:"webrtc_url"`
	RTSPURL        string    `json:"rtsp_url"`
	ProgressSocket string    `json:"-"` // Runtime socket path, not serialized
}

// StreamCreateParams contains parameters for creating a new stream
type StreamCreateParams struct {
	StreamID    string
	DeviceID    string
	Codec       string
	InputFormat string
	Bitrate     *float64 // Optional, in Mbps
	Width       *int     // Optional, video width
	Height      *int     // Optional, video height
	Framerate   *int     // Optional, video framerate
	AudioDevice string   // Optional, ALSA audio device
	Options     []string // Optional, FFmpeg option keys
}

// StreamStatus represents the runtime status of a stream
type StreamStatus struct {
	StreamID  string        `json:"stream_id"`
	Uptime    time.Duration `json:"uptime"`
	StartTime time.Time     `json:"start_time"`
}
