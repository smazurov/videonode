package events

import (
	"github.com/smazurov/videonode/internal/api/models"
	"github.com/smazurov/videonode/internal/hostmetrics"
)

// Event type constants for the bus. These are runtime-only topic keys (never
// serialized), so they may be renumbered freely when types are added or
// removed — but they must stay unique; Subscribe panics on a collision.
const (
	TypeDeviceDiscovery uint32 = iota + 1
	TypeLogEntry
	TypeStreamCrashed
	TypeHeartbeat
	TypePipelineStateChanged
	TypeEntity
	TypeProcesses
	TypeProcessRemoved
)

// Event is implemented by every bus event; Type() routes an event to the
// subscribers keyed on the same code.
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

// ProcessInfo is one supervised-process row pushed on the process event
// stream. Its JSON mirrors the /api/processes ProcessEntry row (minus the
// always-empty source-registry join fields) so the UI keeps poll and push
// results in a single shape. Kind carries the user-facing entity name; the
// API edge normalizes the pipeline's internal "producer" label to "source"
// before this lands on the wire.
type ProcessInfo struct {
	ID           string  `json:"id" doc:"Pool key (e.g. 'source:hdmi0' / 'composer:cam-front'); 'self' for the daemon row"`
	Kind         string  `json:"kind" enum:"source,composer,encoder,daemon" doc:"Entity kind for this stage ('daemon' = the videonode process itself)"`
	StreamID     string  `json:"stream_id,omitempty" doc:"User-facing stream id (empty for shared sources)"`
	State        string  `json:"state" doc:"Pool state: idle/starting/running/stopping/error"`
	PID          int     `json:"pid,omitempty" doc:"OS pid when running; 0 otherwise"`
	StartedAtUS  int64   `json:"started_at_us,omitempty" doc:"Unix microseconds at Start; 0 when never started"`
	RestartCount int     `json:"restart_count,omitempty" doc:"Times the supervisor restarted this stage"`
	LastError    string  `json:"last_error,omitempty" doc:"Most recent error from the supervisor"`
	RSSBytes     int64   `json:"rss_bytes,omitempty" doc:"Resident set size in bytes"`
	CPUPercent   float64 `json:"cpu_percent,omitempty" doc:"CPU usage as percentage (0-100 per core)"`

	// Device-global hardware utilization, on the 'self' (daemon) row only.
	RKMPP []hostmetrics.RKMPPCore  `json:"rkmpp,omitempty" doc:"Per-core Rockchip MPP codec load (host row only)"`
	GPU   *hostmetrics.DevfreqLoad `json:"gpu,omitempty" doc:"Mali GPU devfreq load (host row only)"`
	NPU   *hostmetrics.DevfreqLoad `json:"npu,omitempty" doc:"RKNN NPU devfreq load (host row only)"`
}

// ProcessesEvent carries the current set of supervised processes on the
// dedicated process event stream. Published on every state transition
// (immediately) and on each 2s stats sample while at least one process is
// running, so an idle pipeline produces no traffic.
type ProcessesEvent struct {
	Processes []ProcessInfo `json:"processes" doc:"All supervised pipeline stages with current state + stats"`
	Timestamp string        `json:"timestamp" doc:"Server time when the snapshot was taken"`
}

// Type returns the event type identifier for ProcessesEvent.
func (e ProcessesEvent) Type() uint32 { return TypeProcesses }

// ProcessRemovedEvent fires when a supervised process leaves the pool. A
// removed process emits no further state or stats events, so this is the
// signal for subscribers to drop its row. Carries the same user-facing id as
// the ProcessesEvent rows (normalized at the API edge).
type ProcessRemovedEvent struct {
	ID        string `json:"id" doc:"Pool key of the removed process (e.g. 'source:hdmi0')"`
	Timestamp string `json:"timestamp" doc:"Server time when the process was removed"`
}

// Type returns the event type identifier for ProcessRemovedEvent.
func (e ProcessRemovedEvent) Type() uint32 { return TypeProcessRemoved }

// PipelineStateChangedEvent fires when the daemon-wide pipeline master
// switch is toggled. UI uses it to keep the start/stop button in sync.
type PipelineStateChangedEvent struct {
	Enabled   bool   `json:"enabled" example:"true" doc:"Whether the pipeline master switch is on"`
	Timestamp string `json:"timestamp" example:"2025-01-27T10:30:00Z" doc:"Event timestamp"`
}

// Type returns the event type identifier for PipelineStateChangedEvent.
func (e PipelineStateChangedEvent) Type() uint32 { return TypePipelineStateChanged }
