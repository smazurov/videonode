package streams

import (
	"context"
	"time"

	"github.com/smazurov/videonode/internal/devices"
	"github.com/smazurov/videonode/internal/ffmpeg"
	"github.com/smazurov/videonode/internal/types"
)

// StreamService defines the interface for stream operations.
type StreamService interface {
	CreateStream(ctx context.Context, params StreamCreateParams) (*Stream, error)
	UpdateStream(ctx context.Context, streamID string, params StreamUpdateParams) (*Stream, error)
	UpdatePartial(ctx context.Context, streamID string, patch func(*StreamSpec) error) (*Stream, error)
	SetEnabled(ctx context.Context, streamID string, enabled bool) (bool, error)
	DeleteStream(ctx context.Context, streamID string) error
	RestartStream(ctx context.Context, streamID string) error
	// ReleaseCanvas stops a canvas and resumes its sources as standalone streams (canvas spec preserved, Enabled=false).
	ReleaseCanvas(ctx context.Context, streamID string) error
	// EngageCanvas starts a dormant canvas, claiming its sources.
	EngageCanvas(ctx context.Context, streamID string) error
	GetStream(ctx context.Context, streamID string) (*Stream, error)
	GetStreamSpec(ctx context.Context, streamID string) (*StreamSpec, error)
	ListStreams(ctx context.Context) ([]Stream, error)
	ListStreamsWithSpecs(ctx context.Context) ([]StreamWithSpec, error)
	GetFFmpegCommand(ctx context.Context, streamID string, encoderOverride string) (string, bool, error)

	// Initialization
	LoadStreamsFromConfig() error

	// Process management
	GetProcessManager() StreamProcessManager

	// Device event handling
	BroadcastDeviceDiscovery(action string, device devices.DeviceInfo, timestamp string)

	// ValidationProvider returns the shared validation data accessor backed by the same store.
	ValidationProvider() types.ValidationProvider
}

// StreamWithSpec pairs a runtime Stream with its persisted StreamSpec.
type StreamWithSpec struct {
	Stream Stream
	Spec   StreamSpec
}

// StreamCollector is the interface for stream metrics collectors.
type StreamCollector interface {
	Stop() error
}

// Stream is a video stream's runtime state. Configuration lives in StreamSpec.
type Stream struct {
	ID             string          `json:"stream_id"`
	Enabled        bool            `json:"enabled"`
	StartTime      time.Time       `json:"start_time"`
	ProgressSocket string          `json:"-"`
	Collector      StreamCollector `json:"-"`
	InputsEnabled  map[string]bool `json:"inputs_enabled,omitempty"` // canvas streams only

	OwnedBy string `json:"owned_by,omitempty"` // canvas ID currently owning this stream's device
}

// StreamCreateParams contains parameters for creating a new stream.
type StreamCreateParams struct {
	StreamID    string
	DeviceID    string
	Codec       string
	InputFormat string
	Bitrate     *float64      // Optional, in Mbps
	Width       *int          // Optional, video width
	Height      *int          // Optional, video height
	Framerate   *int          // Optional, video framerate
	AudioDevice string        // Optional, ALSA audio device
	Options     []string      // Optional, FFmpeg option keys
	Canvas      *CanvasConfig // nil for regular streams, non-nil for canvas (composite) streams
	Rotation    int           // Output rotation in degrees (0, 90, 180, 270)
}

// StreamUpdateParams is the legacy fully-populated update payload; prefer UpdatePartial.
type StreamUpdateParams struct {
	Codec               string
	InputFormat         string
	Resolution          string
	FPS                 string
	AudioDevice         string
	Options             []ffmpeg.OptionType
	QualityParams       *types.QualityParams
	CustomFFmpegCommand string
	TestMode            bool
	Canvas              *CanvasConfig
	Perspective         *ffmpeg.PerspectiveConfig
	Vision              *ffmpeg.VisionConfig
	Rotation            int

	Enabled *bool // runtime state, applied only when non-nil; not persisted
}
