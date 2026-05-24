package streams

// Process-manager interface definitions. The canonical implementation
// lives in pipeline_process_manager.go (the Pipeline-backed translation
// layer).

import (
	"time"
)

// ProcessState represents the current state of a stream process.
type ProcessState string

// Process states for stream FFmpeg processes.
const (
	ProcessStateIdle     ProcessState = "idle"
	ProcessStateStarting ProcessState = "starting"
	ProcessStateRunning  ProcessState = "running"
	ProcessStateStopping ProcessState = "stopping"
	ProcessStateError    ProcessState = "error"
)

// ProcessInfo contains information about a stream process.
type ProcessInfo struct {
	StreamID     string
	State        ProcessState
	PID          int
	StartedAt    time.Time
	RestartCount int
	LastError    error
}

// StreamProcessManager manages supervised stream processes. The
// canonical implementation is pipelineProcessManager (translates to
// pipeline.Pipeline.Apply / Delete); tests can substitute a mock.
type StreamProcessManager interface {
	Start(streamID string) error
	Stop(streamID string) error
	Restart(streamID string) error
	GetStatus(streamID string) (*ProcessInfo, error)
	StartAll() error
	StopAll()
	IsRunning(streamID string) bool
	IsCrashed(streamID string) bool
	// CaptureSourceSnapshot pulls a raw NV12-derived JPEG snapshot from a
	// source producer via the pipelinectl Snapshot RPC.
	CaptureSourceSnapshot(sourceID string) ([]byte, error)
}
