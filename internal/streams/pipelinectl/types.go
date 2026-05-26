// Package pipelinectl is the daemon-side gRPC client manager for the
// native binaries the daemon supervises — videonode-source instances
// (kind "source") and videonode-composer instances (kind "composer").
// The daemon dials each spawned binary's per-instance UDS, calls
// Describe() to seed identity, then issues unary RPCs (SetFormat,
// SetCanvas, …) and (for sources) subscribes to StreamStatus.
//
// Wire format: gRPC over SOCK_STREAM UDS, see proto/control/*.proto.
package pipelinectl

// SetFormatParams is the request payload for the "set_format" command
// sent from the daemon to a client. All fields except FPS are required;
// FPS=0 means "driver decides".
type SetFormatParams struct {
	FourCC string `json:"fourcc"`
	W      uint32 `json:"w"`
	H      uint32 `json:"h"`
	FPS    uint32 `json:"fps"`
}

// SetFormatResult is the response payload for a successful "set_format".
type SetFormatResult struct {
	Applied bool `json:"applied"`
}

// StatusParams is the payload of an unsolicited "status" notification
// sent by a client on health change, consumer-count change, or once per
// second as a heartbeat. Mirrors the shape produced by
// composer/src/videonode_source_main.cpp:build_status_params.
type StatusParams struct {
	DeviceID    string `json:"device_id"`
	TimestampMs int64  `json:"ts_ms"`
	// StartedAtUs is the producer process's start time in Unix
	// microseconds, stamped daemon-side from pipeline.Pool.GetStatus.
	// 0 when the process hasn't been started (e.g., source isn't yet
	// supervised). UI consumers use this to derive uptime.
	StartedAtUs int64               `json:"started_at_us,omitempty"`
	Health      string              `json:"health"`
	Device      SourceDeviceInfo    `json:"device"`
	Signal      SourceSignalInfo    `json:"signal"`
	Format      SourceFormatInfo    `json:"format"`
	Broadcast   SourceBroadcastInfo `json:"broadcast"`
	Consumers   SourceConsumersInfo `json:"consumers"`
}

// SourceDeviceInfo describes the V4L2 device path + capability flags.
type SourceDeviceInfo struct {
	Path        string `json:"path"`
	Multiplanar bool   `json:"multiplanar"`
}

// SourceSignalInfo describes HDMI signal state (where applicable).
type SourceSignalInfo struct {
	HasDvTimings bool   `json:"has_dv_timings"`
	CablePresent bool   `json:"cable_present"`
	SignalLocked bool   `json:"signal_locked"`
	DvTimings    string `json:"dv_timings"`
}

// SourceFormatInfo describes the currently-negotiated capture format.
type SourceFormatInfo struct {
	FourCC  string `json:"fourcc"`
	W       uint32 `json:"w"`
	H       uint32 `json:"h"`
	FPS     uint32 `json:"fps"`
	Buffers uint32 `json:"buffers"`
	Mode    string `json:"mode"`
}

// SourceBroadcastInfo describes the dma-buf publish rate + counters.
type SourceBroadcastInfo struct {
	TargetFPS         uint32 `json:"target_fps"`
	RealFrames        uint64 `json:"real_frames"`
	PlaceholderFrames uint64 `json:"placeholder_frames"`
	LastSeq           uint32 `json:"last_seq"`
}

// SourceConsumersInfo summarizes the SCM_RIGHTS consumer set: live count plus
// per-consumer frame counters (both live and evicted).
type SourceConsumersInfo struct {
	Count   int                   `json:"count"`
	Live    []SourceConsumerEntry `json:"live"`
	Evicted []SourceConsumerEntry `json:"evicted"`
}

// SourceConsumerEntry is one row of the consumer telemetry.
type SourceConsumerEntry struct {
	FD             int    `json:"fd"`
	FramesSent     uint64 `json:"frames_sent"`
	FramesDropped  uint64 `json:"frames_dropped"`
	EvictedAtFrame uint64 `json:"evicted_at_frame,omitempty"`
}

// ============================================================
// Composer-side method params (daemon -> videonode-composer).
//
// Composer is daemon-driven: argv is just --drm-device / --ctl-connect /
// --composer-id. The daemon pushes everything else over the same
// control channel pipelinectl already runs for videonode-source.
// ============================================================

// SetCanvasParams configures the composer's output canvas. Must arrive
// before composer can render anything useful.
type SetCanvasParams struct {
	W   uint32 `json:"w"`
	H   uint32 `json:"h"`
	FPS uint32 `json:"fps"`
}

// SetSourceParams binds a slot to a SCM-publishing source. SourceID is
// the stable identity the daemon uses elsewhere (matching the source's
// own --device-id). ScmPath is the Unix-socket path that the source's
// scm_rights_producer is listening on (e.g. /tmp/vn-bus-hdmi.sock).
type SetSourceParams struct {
	Slot     string `json:"slot"`
	SourceID string `json:"source_id"`
	ScmPath  string `json:"scm_path"`
	Width    uint32 `json:"width"`
	Height   uint32 `json:"height"`
	FPS      uint32 `json:"fps"`
}

// ClearSourceParams unbinds a slot.
type ClearSourceParams struct {
	Slot string `json:"slot"`
}

// SetLayoutParams replaces the whole layout (set of placed slots) for
// the composer. Order doesn't matter; the composer keys by slot name.
type SetLayoutParams struct {
	Slots []LayoutSlotEntry `json:"slots"`
}

// LayoutSlotEntry is the placement of one slot on the canvas. Coords are
// in canvas pixels; x/y may be negative (off-canvas slots are clipped).
type LayoutSlotEntry struct {
	Slot     string `json:"slot"`
	X        int32  `json:"x"`
	Y        int32  `json:"y"`
	W        int32  `json:"w"`
	H        int32  `json:"h"`
	Rotation int32  `json:"rotation,omitempty"`
}

// EffectParams is one effect in a per-source effect list. Tagged-union by
// Type; today only "perspective" is fully populated, future types (crop,
// bbox) will use the same shape with their own param fields. Unknown
// Types are recognized by the composer as "log + skip" — wire-compat for
// new effects without composer rebuilds.
type EffectParams struct {
	Type           string    `json:"type"`
	Corners        [4][2]int `json:"corners,omitempty"`    // perspective
	SnapshotWidth  int       `json:"snapshot_w,omitempty"` // perspective
	SnapshotHeight int       `json:"snapshot_h,omitempty"` // perspective
}

// SetEffectsParams atomically replaces the effect list bound to a
// source_id. Order is significant for the composer (applied in array
// order, but today only one perspective wins).
type SetEffectsParams struct {
	SourceID string         `json:"source_id"`
	Effects  []EffectParams `json:"effects"`
}

// SetSourceStateParams notifies composer that a source's collapsed
// health state has changed. Composer applies the user warp only when
// State is "live" or "transitioning"; for "placeholder" it falls back
// to identity so any NO-SIGNAL overlay stays readable.
type SetSourceStateParams struct {
	SourceID string `json:"source_id"`
	State    string `json:"state"` // "live" | "transitioning" | "placeholder"
}
