package streams

import "time"

// Stream represents a video stream
type Stream struct {
	ID             string    `json:"stream_id"`
	DeviceID       string    `json:"device_id"`
	Codec          string    `json:"codec"`
	StartTime      time.Time `json:"start_time"`
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

// StreamUpdateParams contains parameters for updating an existing stream
type StreamUpdateParams struct {
	Codec               *string  // Optional, video codec
	InputFormat         *string  // Optional, input format
	Bitrate             *float64 // Optional, in Mbps
	Width               *int     // Optional, video width
	Height              *int     // Optional, video height
	Framerate           *int     // Optional, video framerate
	AudioDevice         *string  // Optional, ALSA audio device
	Options             []string // Optional, FFmpeg option keys
	CustomFFmpegCommand *string  // Optional, custom FFmpeg command override
	TestMode            *bool    // Optional, enable test pattern mode
}

// StreamStatus represents the runtime status of a stream
type StreamStatus struct {
	StreamID  string    `json:"stream_id"`
	StartTime time.Time `json:"start_time"`
}
