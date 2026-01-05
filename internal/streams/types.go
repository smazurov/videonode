package streams

import (
	"context"
	"time"

	"github.com/smazurov/videonode/internal/devices"
)

// StreamService defines the interface for stream operations.
type StreamService interface {
	CreateStream(ctx context.Context, params StreamCreateParams) (*Stream, error)
	UpdateStream(ctx context.Context, streamID string, params StreamUpdateParams) (*Stream, error)
	DeleteStream(ctx context.Context, streamID string) error
	RestartStream(ctx context.Context, streamID string) error
	GetStream(ctx context.Context, streamID string) (*Stream, error)
	GetStreamSpec(ctx context.Context, streamID string) (*StreamSpec, error)
	ListStreams(ctx context.Context) ([]Stream, error)
	GetFFmpegCommand(ctx context.Context, streamID string, encoderOverride string) (string, bool, error)

	// Initialization
	LoadStreamsFromConfig() error

	// Process management
	GetProcessManager() StreamProcessManager

	// Device event handling
	BroadcastDeviceDiscovery(action string, device devices.DeviceInfo, timestamp string)
}

// Stream represents a video stream's runtime state
// Configuration is stored separately in StreamSpec.
type Stream struct {
	ID             string    `json:"stream_id"`
	Enabled        bool      `json:"enabled"`    // Device online/offline state, set by monitoring
	StartTime      time.Time `json:"start_time"` // When stream was started
	ProgressSocket string    `json:"-"`          // Runtime socket path, not serialized
}

// StreamCreateParams contains parameters for creating a new stream.
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

// StreamUpdateParams contains parameters for updating an existing stream.
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
	Enabled             *bool    // Optional, manual override of runtime enabled state
}
