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
	TypeHeartbeat
	TypeSourceStatus
	TypeStageStateChanged // per-stage (Source/Composer/Encoder) lifecycle event
	TypePipelineStateChanged
	TypeSourceCreated
	TypeSourceUpdated
	TypeSourceDeleted
	TypeComposerCreated
	TypeComposerUpdated
	TypeComposerDeleted
	TypeComposerLayoutChanged
	// TypeEntity is the uniform entity envelope (see EntityEvent in
	// registry.go) that replaces all per-action structs above. Old
	// constants are kept during the dual-publish migration; they're
	// removed in Step 5 of the live-sync rewire.
	TypeEntity
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

// SourcePayload mirrors the canonical Source entity shape (see
// internal/streams/pipeline/source.go). Defined inline to avoid an
// events → pipeline import cycle; integrator may swap to models.SourceData
// once B5 lands.
type SourcePayload struct {
	ID        string                 `json:"id" doc:"Source identifier"`
	Device    string                 `json:"device,omitempty" doc:"Stable device identifier"`
	TestMode  bool                   `json:"test_mode,omitempty" doc:"Source uses the RPC test-pattern producer"`
	Format    *SourceFormatEventBody `json:"format,omitempty" doc:"V4L2 capture format the daemon pushed to the source"`
	CreatedAt string                 `json:"created_at,omitempty" doc:"RFC3339 creation timestamp"`
	UpdatedAt string                 `json:"updated_at,omitempty" doc:"RFC3339 last-update timestamp"`
}

// SourceFormatEventBody mirrors the persisted V4L2 capture format on
// SSE source events. Defined inline here to avoid an events → api/models
// import cycle.
type SourceFormatEventBody struct {
	FormatName string `json:"format_name" doc:"Lowercase video format name"`
	Width      uint32 `json:"width" doc:"Capture width in pixels"`
	Height     uint32 `json:"height" doc:"Capture height in pixels"`
	FPS        uint32 `json:"fps,omitempty" doc:"Capture framerate; 0 = driver default"`
}

// SourceCreatedEvent fires when a source is added.
type SourceCreatedEvent struct {
	SourceID  string        `json:"source_id" doc:"Created source identifier"`
	Source    SourcePayload `json:"source" doc:"Created source data"`
	Timestamp string        `json:"timestamp" doc:"RFC3339 server time"`
}

// Type returns the event type identifier for SourceCreatedEvent.
func (e SourceCreatedEvent) Type() uint32 { return TypeSourceCreated }

// SourceUpdatedEvent fires when a source's spec changes.
type SourceUpdatedEvent struct {
	SourceID  string        `json:"source_id" doc:"Updated source identifier"`
	Source    SourcePayload `json:"source" doc:"Source data after the update"`
	Timestamp string        `json:"timestamp" doc:"RFC3339 server time"`
}

// Type returns the event type identifier for SourceUpdatedEvent.
func (e SourceUpdatedEvent) Type() uint32 { return TypeSourceUpdated }

// SourceDeletedEvent fires when a source is removed.
type SourceDeletedEvent struct {
	SourceID  string `json:"source_id" doc:"Deleted source identifier"`
	Timestamp string `json:"timestamp" doc:"RFC3339 server time"`
}

// Type returns the event type identifier for SourceDeletedEvent.
func (e SourceDeletedEvent) Type() uint32 { return TypeSourceDeleted }

// ComposerPayload mirrors the canonical Composer entity shape (see
// internal/streams/pipeline/composer.go). Defined inline to avoid an
// events → pipeline import cycle; integrator may swap to models.ComposerData
// once B6 lands.
type ComposerPayload struct {
	ID        string                 `json:"id" doc:"Composer identifier"`
	Canvas    ComposerCanvasDims     `json:"canvas" doc:"Canvas dimensions"`
	Inputs    []ComposerInputPayload `json:"inputs,omitempty" doc:"Inputs sourced into the composer"`
	Layout    []ComposerLayoutSlot   `json:"layout,omitempty" doc:"Layout slots placed on the canvas"`
	CreatedAt string                 `json:"created_at,omitempty" doc:"RFC3339 creation timestamp"`
	UpdatedAt string                 `json:"updated_at,omitempty" doc:"RFC3339 last-update timestamp"`
}

// ComposerCanvasDims is the canvas size and render rate for a composer
// payload. FPS is omitted when unset; consumers fall back to the daemon
// default.
type ComposerCanvasDims struct {
	W   int `json:"w" doc:"Canvas width in pixels"`
	H   int `json:"h" doc:"Canvas height in pixels"`
	FPS int `json:"fps,omitempty" doc:"Canvas frame rate (0 = daemon default)"`
}

// ComposerInputPayload mirrors a composer input ref + optional effect.
type ComposerInputPayload struct {
	Ref    string          `json:"ref" doc:"Upstream ref, e.g. 'source:<id>'"`
	Effect *ComposerEffect `json:"effect,omitempty" doc:"Optional per-input effect"`
}

// ComposerEffect describes a single effect applied to a composer input.
type ComposerEffect struct {
	Type      string    `json:"type" doc:"Effect type, e.g. 'perspective'"`
	Corners   [4][2]int `json:"corners,omitempty" doc:"Perspective corners when type='perspective'"`
	SnapshotW int       `json:"snapshot_w,omitempty" doc:"Source pixel width the corners are expressed in"`
	SnapshotH int       `json:"snapshot_h,omitempty" doc:"Source pixel height the corners are expressed in"`
}

// ComposerLayoutSlot describes one slot's geometry on the canvas.
type ComposerLayoutSlot struct {
	Input string `json:"input" doc:"Matches ComposerInputPayload.Ref"`
	X     int    `json:"x" doc:"Slot X in canvas pixels"`
	Y     int    `json:"y" doc:"Slot Y in canvas pixels"`
	W     int    `json:"w" doc:"Slot width in canvas pixels"`
	H     int    `json:"h" doc:"Slot height in canvas pixels"`
}

// ComposerCreatedEvent fires when a composer is added.
type ComposerCreatedEvent struct {
	ComposerID string          `json:"composer_id" doc:"Created composer identifier"`
	Composer   ComposerPayload `json:"composer" doc:"Created composer data"`
	Timestamp  string          `json:"timestamp" doc:"RFC3339 server time"`
}

// Type returns the event type identifier for ComposerCreatedEvent.
func (e ComposerCreatedEvent) Type() uint32 { return TypeComposerCreated }

// ComposerUpdatedEvent fires when a composer's spec changes (other than
// layout-only edits, which use ComposerLayoutChangedEvent).
type ComposerUpdatedEvent struct {
	ComposerID string          `json:"composer_id" doc:"Updated composer identifier"`
	Composer   ComposerPayload `json:"composer" doc:"Composer data after the update"`
	Timestamp  string          `json:"timestamp" doc:"RFC3339 server time"`
}

// Type returns the event type identifier for ComposerUpdatedEvent.
func (e ComposerUpdatedEvent) Type() uint32 { return TypeComposerUpdated }

// ComposerDeletedEvent fires when a composer is removed.
type ComposerDeletedEvent struct {
	ComposerID string `json:"composer_id" doc:"Deleted composer identifier"`
	Timestamp  string `json:"timestamp" doc:"RFC3339 server time"`
}

// Type returns the event type identifier for ComposerDeletedEvent.
func (e ComposerDeletedEvent) Type() uint32 { return TypeComposerDeleted }

// ComposerLayoutChangedEvent fires on a live layout edit (PATCH
// /api/composers/{id}/layout). Carries only the new layout slots so the
// UI can apply incremental updates without a full composer refetch.
type ComposerLayoutChangedEvent struct {
	ComposerID string               `json:"composer_id" doc:"Composer whose layout changed"`
	Layout     []ComposerLayoutSlot `json:"layout" doc:"New layout slots"`
	Timestamp  string               `json:"timestamp" doc:"RFC3339 server time"`
}

// Type returns the event type identifier for ComposerLayoutChangedEvent.
func (e ComposerLayoutChangedEvent) Type() uint32 { return TypeComposerLayoutChanged }

// StageStateChangedEvent fires when a pipeline stage (Producer,
// Composer, or Encoder) transitions in or out of Running. Replaces the
// stream-level StreamStateChangedEvent for callers that care about
// per-stage health (process-manager UI, alerting). Both events fire —
// old subscribers can ignore this; new ones prefer it.
type StageStateChangedEvent struct {
	StreamID  string `json:"stream_id" doc:"User-facing stream this stage belongs to (empty for shared producers)"`
	StageID   string `json:"stage_id" doc:"Pool key, e.g. 'producer:hdmi0' or 'composer:cam-front'"`
	StageKind string `json:"stage_kind" doc:"'producer' | 'composer' | 'encoder'"`
	OldState  string `json:"old_state" doc:"Prior process state"`
	NewState  string `json:"new_state" doc:"New process state"`
	PID       int    `json:"pid,omitempty" doc:"OS pid when running; 0 otherwise"`
	Error     string `json:"error,omitempty" doc:"Error message when NewState is 'error'"`
	Timestamp string `json:"timestamp" doc:"RFC3339 server time"`
}

// Type returns the event type identifier for StageStateChangedEvent.
func (e StageStateChangedEvent) Type() uint32 { return TypeStageStateChanged }

// PipelineStateChangedEvent fires when the daemon-wide pipeline master
// switch is toggled. UI uses it to keep the start/stop button in sync.
type PipelineStateChangedEvent struct {
	Enabled   bool   `json:"enabled" example:"true" doc:"Whether the pipeline master switch is on"`
	Timestamp string `json:"timestamp" example:"2025-01-27T10:30:00Z" doc:"Event timestamp"`
}

// Type returns the event type identifier for PipelineStateChangedEvent.
func (e PipelineStateChangedEvent) Type() uint32 { return TypePipelineStateChanged }
