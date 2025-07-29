package streams

import "time"

// Stream represents a video stream
type Stream struct {
	ID        string    `json:"stream_id"`
	DeviceID  string    `json:"device_id"`
	Codec     string    `json:"codec"`
	StartTime time.Time `json:"start_time"`
	WebRTCURL string    `json:"webrtc_url"`
	RTSPURL   string    `json:"rtsp_url"`
}

// StreamCreateParams contains parameters for creating a new stream
type StreamCreateParams struct {
	StreamID  string
	DeviceID  string
	Codec     string
	Bitrate   *int // Optional, in kbps
	Width     *int // Optional, video width
	Height    *int // Optional, video height
	Framerate *int // Optional, video framerate
}

// StreamStatus represents the runtime status of a stream
type StreamStatus struct {
	StreamID  string        `json:"stream_id"`
	Uptime    time.Duration `json:"uptime"`
	StartTime time.Time     `json:"start_time"`
}

// StreamMetrics contains performance metrics for a stream
type StreamMetrics struct {
	StreamID     string  `json:"stream_id"`
	FPS          float64 `json:"fps"`
	BitrateKbps  float64 `json:"bitrate_kbps"`
	DroppedFrames int64  `json:"dropped_frames"`
}