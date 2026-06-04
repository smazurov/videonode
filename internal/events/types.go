package events

import (
	"github.com/smazurov/videonode/internal/api/models"
)

// Event type constants for kelindar/event. These are runtime-only topic keys
// (never serialized), so they may be renumbered freely when types are added
// or removed.
const (
	TypeDeviceDiscovery uint32 = iota + 1
	TypeLogEntry
	TypeStreamCrashed
	TypeHeartbeat
	TypePipelineStateChanged
	TypeEntity
)

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

// HeartbeatEvent keeps SSE connections open through proxies and lets the
// client confirm the stream is live even when no domain events fire.
type HeartbeatEvent struct {
	Timestamp string `json:"timestamp" doc:"Server time at heartbeat"`
}

// Type returns the event type identifier for HeartbeatEvent.
func (e HeartbeatEvent) Type() uint32 { return TypeHeartbeat }

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
// Used by the device detector to check HDMI signal state.
type StreamCrashedEvent struct {
	StreamID  string `json:"stream_id"`
	DeviceID  string `json:"device_id"`
	Timestamp string `json:"timestamp"`
}

// Type returns the event type identifier for StreamCrashedEvent.
func (e StreamCrashedEvent) Type() uint32 { return TypeStreamCrashed }

// PipelineStateChangedEvent fires when the daemon-wide pipeline master
// switch is toggled. UI uses it to keep the start/stop button in sync.
type PipelineStateChangedEvent struct {
	Enabled   bool   `json:"enabled" example:"true" doc:"Whether the pipeline master switch is on"`
	Timestamp string `json:"timestamp" example:"2025-01-27T10:30:00Z" doc:"Event timestamp"`
}

// Type returns the event type identifier for PipelineStateChangedEvent.
func (e PipelineStateChangedEvent) Type() uint32 { return TypePipelineStateChanged }
