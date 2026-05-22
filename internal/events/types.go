package events

import (
	"github.com/smazurov/videonode/internal/api/models"
	"github.com/smazurov/videonode/internal/streams/pipelinectl"
)

// Event type constants for kelindar/event.
const (
	TypeDeviceDiscovery uint32 = iota + 1
	TypeStreamCreated
	TypeStreamUpdated
	TypeStreamDeleted
	TypeStreamStateChanged
	TypeStreamMetrics
	TypeLogEntry
	TypeStreamCrashed
	TypeCanvasRestarted
	TypeHeartbeat
	TypeSourceStatus
)

// SourceStatusEvent carries a status snapshot published by a
// videonode-source sidecar over the control plane. Emitted on health
// changes, consumer-count changes, and ~1 Hz heartbeat.
type SourceStatusEvent struct {
	DeviceID  string                   `json:"device_id" doc:"Stable device identifier the snapshot describes"`
	Status    pipelinectl.StatusParams `json:"status" doc:"Full status snapshot from the sidecar"`
	Timestamp string                   `json:"timestamp" doc:"Server time when received"`
}

// Type returns the event type identifier for SourceStatusEvent.
func (e SourceStatusEvent) Type() uint32 { return TypeSourceStatus }

// HeartbeatEvent keeps SSE connections open through proxies and lets the
// client confirm the stream is live even when no domain events fire.
type HeartbeatEvent struct {
	Timestamp string `json:"timestamp" doc:"Server time at heartbeat"`
}

// Type returns the event type identifier for HeartbeatEvent.
func (e HeartbeatEvent) Type() uint32 { return TypeHeartbeat }

// Event interface required by kelindar/event.
type Event interface {
	Type() uint32
}

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
	Action    string `json:"action,omitempty" example:"running" doc:"Action: enabled, disabled, running"`
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

// CanvasRestartedEvent is published when a canvas stream is restarted in
// response to a source stream's config change. The UI uses this to refresh
// the canvas card (layout may have changed based on the source's new
// effective aspect ratio) without waiting for the next metrics tick.
type CanvasRestartedEvent struct {
	CanvasID  string            `json:"canvas_id" example:"mycanvas" doc:"Canvas stream identifier that was restarted"`
	TriggerID string            `json:"trigger_id" example:"cam1" doc:"Source stream whose update triggered the restart"`
	Canvas    models.StreamData `json:"canvas" doc:"Full canvas stream data after restart"`
	Timestamp string            `json:"timestamp" example:"2025-01-27T10:30:00Z" doc:"Event timestamp"`
}

// Type returns the event type identifier for CanvasRestartedEvent.
func (e CanvasRestartedEvent) Type() uint32 { return TypeCanvasRestarted }
