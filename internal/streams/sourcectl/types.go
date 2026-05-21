// Package sourcectl is the daemon-side control plane for videonode-source
// sidecars. It owns one well-known Unix-domain control socket; each
// sidecar dials in, identifies itself with its stable device ID, then
// exchanges JSON-RPC 2.0 frames bidirectionally with the daemon.
//
// Wire format: newline-delimited JSON-RPC 2.0 over SOCK_STREAM UDS.
// Library: github.com/creachadair/jrpc2 with channel.Line framing and
// server-side AllowPush so we can send commands and accept unsolicited
// status notifications on the same connection.
package sourcectl

// IdentifyParams is sent by a sidecar as its first message after
// connecting. Both fields are required.
type IdentifyParams struct {
	DeviceID string `json:"device_id"`
	PID      int    `json:"pid"`
	Version  string `json:"version,omitempty"`
}

// SetFormatParams is the request payload for the "set_format" command
// sent from the daemon to a sidecar. All fields except FPS are required;
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
// sent by a sidecar on health change, consumer-count change, or once per
// second as a heartbeat. Mirrors the shape produced by
// composer/src/videonode_source_main.cpp:build_status_params.
type StatusParams struct {
	DeviceID    string              `json:"device_id"`
	TimestampMs int64               `json:"ts_ms"`
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
