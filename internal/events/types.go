package events

import "github.com/smazurov/videonode/internal/api/models"

// Event type constants for kelindar/event.
const (
	TypeCaptureSuccess uint32 = iota + 1
	TypeCaptureError
	TypeDeviceDiscovery
	TypeStreamCreated
	TypeStreamUpdated
	TypeStreamDeleted
	TypeStreamStateChanged
	TypeStreamMetrics
	TypeLogEntry
	TypeStreamCrashed
)

// Event interface required by kelindar/event.
type Event interface {
	Type() uint32
}

// CaptureSuccessEvent represents a successful screenshot capture.
type CaptureSuccessEvent struct {
	DevicePath string `json:"device_path" example:"/dev/video0" doc:"Path to the video device"`
	Message    string `json:"message" example:"Screenshot captured successfully" doc:"Message"`
	ImageData  string `json:"image_data" doc:"Base64-encoded screenshot image"`
	Timestamp  string `json:"timestamp" example:"2025-01-27T10:30:00Z" doc:"Capture timestamp"`
}

// Type returns the event type identifier for CaptureSuccessEvent.
func (e CaptureSuccessEvent) Type() uint32 { return TypeCaptureSuccess }

// CaptureErrorEvent represents a failed screenshot capture.
type CaptureErrorEvent struct {
	DevicePath string `json:"device_path" example:"/dev/video0" doc:"Path to the video device"`
	Message    string `json:"message" example:"Screenshot capture failed" doc:"Error message"`
	Error      string `json:"error" example:"Device not found" doc:"Detailed error description"`
	Timestamp  string `json:"timestamp" example:"2025-01-27T10:30:00Z" doc:"Error timestamp"`
}

// Type returns the event type identifier for CaptureErrorEvent.
func (e CaptureErrorEvent) Type() uint32 { return TypeCaptureError }

// DeviceDiscoveryEvent represents device hotplug events.
type DeviceDiscoveryEvent struct {
	models.DeviceInfo
	Action    string `json:"action" example:"added" doc:"Action type: added, removed, changed"`
	Timestamp string `json:"timestamp" example:"2025-01-27T10:30:00Z" doc:"Event timestamp"`
}

// Type returns the event type identifier for DeviceDiscoveryEvent.
func (e DeviceDiscoveryEvent) Type() uint32 { return TypeDeviceDiscovery }

// StreamCreatedEvent represents a successful stream creation.
type StreamCreatedEvent struct {
	Stream    models.StreamData `json:"stream" doc:"Created stream data"`
	Action    string            `json:"action" example:"created" doc:"Action type"`
	Timestamp string            `json:"timestamp" example:"2025-01-27T10:30:00Z" doc:"Event timestamp"`
}

// Type returns the event type identifier for StreamCreatedEvent.
func (e StreamCreatedEvent) Type() uint32 { return TypeStreamCreated }

// StreamDeletedEvent represents a successful stream deletion.
type StreamDeletedEvent struct {
	StreamID  string `json:"stream_id" example:"stream-001" doc:"Deleted stream identifier"`
	Action    string `json:"action" example:"deleted" doc:"Action type"`
	Timestamp string `json:"timestamp" example:"2025-01-27T10:30:00Z" doc:"Event timestamp"`
}

// Type returns the event type identifier for StreamDeletedEvent.
func (e StreamDeletedEvent) Type() uint32 { return TypeStreamDeleted }

// StreamUpdatedEvent represents a successful stream update.
type StreamUpdatedEvent struct {
	Stream    models.StreamData `json:"stream" doc:"Updated stream data"`
	Action    string            `json:"action" example:"updated" doc:"Action type"`
	Timestamp string            `json:"timestamp" example:"2025-01-27T10:30:00Z" doc:"Event timestamp"`
}

// Type returns the event type identifier for StreamUpdatedEvent.
func (e StreamUpdatedEvent) Type() uint32 { return TypeStreamUpdated }

// StreamStateChangedEvent represents a change in stream enabled state
// Used for LED control and other reactive subsystems.
type StreamStateChangedEvent struct {
	StreamID  string `json:"stream_id" example:"stream-001" doc:"Stream identifier"`
	Enabled   bool   `json:"enabled" example:"true" doc:"Whether stream is enabled"`
	Timestamp string `json:"timestamp" example:"2025-01-27T10:30:00Z" doc:"Event timestamp"`
}

// Type returns the event type identifier for StreamStateChangedEvent.
func (e StreamStateChangedEvent) Type() uint32 { return TypeStreamStateChanged }

// GetStreamID implements the StreamStateEvent interface for LED manager.
func (e StreamStateChangedEvent) GetStreamID() string {
	return e.StreamID
}

// IsEnabled implements the StreamStateEvent interface for LED manager.
func (e StreamStateChangedEvent) IsEnabled() bool {
	return e.Enabled
}

// StreamMetricsEvent represents FFmpeg stream metrics.
type StreamMetricsEvent struct {
	EventType       string `json:"type"`
	StreamID        string `json:"stream_id"`
	FPS             string `json:"fps"`
	DroppedFrames   string `json:"dropped_frames"`
	DuplicateFrames string `json:"duplicate_frames"`
}

// Type returns the event type identifier for StreamMetricsEvent.
func (e StreamMetricsEvent) Type() uint32 { return TypeStreamMetrics }

// LogEntryEvent represents a log entry for SSE streaming.
type LogEntryEvent struct {
	Seq        uint64         `json:"seq" example:"42" doc:"Monotonic sequence number for deduplication"`
	Timestamp  string         `json:"timestamp" example:"2025-01-09T10:30:00.123Z" doc:"Log timestamp"`
	Level      string         `json:"level" example:"info" doc:"Log level"`
	Module     string         `json:"module" example:"api" doc:"Source module"`
	Message    string         `json:"message" doc:"Log message"`
	Attributes map[string]any `json:"attributes,omitempty" doc:"Structured log attributes"`
}

// Type returns the event type identifier for LogEntryEvent.
func (e LogEntryEvent) Type() uint32 { return TypeLogEntry }

// StreamCrashedEvent is published when an FFmpeg stream crashes.
// Used by device detector to check HDMI signal state.
type StreamCrashedEvent struct {
	StreamID  string `json:"stream_id"`
	DeviceID  string `json:"device_id"`
	Timestamp string `json:"timestamp"`
}

// Type returns the event type identifier for StreamCrashedEvent.
func (e StreamCrashedEvent) Type() uint32 { return TypeStreamCrashed }
