// Package streams: process-manager interface definitions, extracted
// from the legacy process_manager.go so the StreamProcessManager
// contract survives the legacy file's deletion. The concrete
// implementation lives in pipeline_process_manager.go (the
// Pipeline-backed translation layer).
package streams

import (
	"time"

	"github.com/smazurov/videonode/internal/ffmpeg"
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

// StreamProcessManager manages supervised stream processes. Post-rip
// the canonical implementation is pipelineProcessManager (translates
// to pipeline.Pipeline.Apply / Delete); tests can substitute a mock.
type StreamProcessManager interface {
	Start(streamID string) error
	Stop(streamID string) error
	Restart(streamID string) error
	// RestartCanvas: in the unified pipeline model, equivalent to
	// Restart for any stream — canvases are just streams with N>1.
	// Kept on the interface for API back-compat.
	RestartCanvas(canvasID string) error
	// ReleaseCanvas: no-op in the unified pipeline model — canvases
	// are always engaged while their spec exists in the store. Kept
	// on the interface for API back-compat.
	ReleaseCanvas(canvasID string) error
	GetStatus(streamID string) (*ProcessInfo, error)
	StartAll() error
	StopAll()
	IsRunning(streamID string) bool
	IsCrashed(streamID string) bool
	// CaptureRawSnapshot pulls a snapshot from the source's
	// pipelinectl Snapshot RPC (post-rip; legacy vision-pipe path is
	// gone with the producer dma-buf fan-out replacement).
	CaptureRawSnapshot(sourceStreamID string) ([]byte, error)
	OwnedBy(sourceStreamID string) string
	// CanvasOwner: in the unified pipeline model, same as OwnedBy —
	// kept on the interface so the perspective-PATCH path (api/streams.go)
	// continues to compile without touching that call site yet.
	CanvasOwner(sourceStreamID string) string
	// PushComposerPerspective forwards a perspective update to the
	// composer over pipelinectl WITHOUT a process restart. Returns
	// (true, nil) when delivered live; (false, nil) when no composer
	// is connected; (false, err) when the dispatch errored.
	PushComposerPerspective(streamID, sourceID string, persp *ffmpeg.PerspectiveConfig) (bool, error)
}
