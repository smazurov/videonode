package streams

import (
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/smazurov/videonode/internal/events"
	"github.com/smazurov/videonode/internal/ffmpeg"
	"github.com/smazurov/videonode/internal/logging"
	"github.com/smazurov/videonode/internal/process"
)

// ProcessState represents the current state of a stream process.
type ProcessState string

// Process states for stream FFmpeg processes.
const (
	ProcessStateIdle     ProcessState = "idle"     // Not running
	ProcessStateStarting ProcessState = "starting" // Being started
	ProcessStateRunning  ProcessState = "running"  // Active and streaming
	ProcessStateStopping ProcessState = "stopping" // Being stopped
	ProcessStateError    ProcessState = "error"    // Failed to start/crashed
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

// StreamProcessManager manages FFmpeg processes for all streams.
type StreamProcessManager interface {
	// Start starts the FFmpeg process for a stream.
	Start(streamID string) error

	// Stop gracefully stops the FFmpeg process for a stream.
	Stop(streamID string) error

	// Restart stops and restarts the FFmpeg process with new config.
	Restart(streamID string) error

	// GetStatus returns the current state of a stream's process.
	GetStatus(streamID string) (*ProcessInfo, error)

	// StartAll starts all enabled streams. Called on daemon startup.
	StartAll() error

	// StopAll gracefully stops all running processes. Called on shutdown.
	StopAll()

	// IsRunning checks if a stream's process is currently running.
	IsRunning(streamID string) bool
}

// streamProcessManager wraps process.Pool with stream-specific behavior.
type streamProcessManager struct {
	pool      process.Pool
	store     Store
	processor *processor
	eventBus  *events.Bus
	logger    *slog.Logger
}

// ProcessManagerOptions contains options for creating a StreamProcessManager.
type ProcessManagerOptions struct {
	Store     Store
	Processor *processor
	EventBus  *events.Bus
}

// NewStreamProcessManager creates a new StreamProcessManager.
func NewStreamProcessManager(opts *ProcessManagerOptions) StreamProcessManager {
	logger := logging.GetLogger("process_manager")

	spm := &streamProcessManager{
		store:     opts.Store,
		processor: opts.Processor,
		eventBus:  opts.EventBus,
		logger:    logger,
	}

	spm.pool = process.NewPool(&process.PoolOptions{
		Logger:           logger,
		CommandProvider:  spm.generateCommand,
		OnStateChange:    spm.onStateChange,
		ConfigureProcess: spm.configureProcess,
	})

	return spm
}

// generateCommand generates FFmpeg command for a stream.
func (m *streamProcessManager) generateCommand(streamID string) (string, error) {
	processed, err := m.processor.processStream(streamID)
	if err != nil {
		return "", err
	}
	return processed.FFmpegCommand, nil
}

// onStateChange handles state transitions for event emission.
func (m *streamProcessManager) onStateChange(id string, _, newState process.State, _ error) {
	if m.eventBus == nil {
		return
	}

	if newState == process.StateRunning {
		m.eventBus.Publish(events.StreamStateChangedEvent{
			StreamID:  id,
			Enabled:   true,
			Timestamp: time.Now().Format(time.RFC3339),
		})
	}
}

// configureProcess sets up FFmpeg-specific log parsing.
func (m *streamProcessManager) configureProcess(_ string, proc *process.Process) {
	proc.SetLogParser(logging.GetLogger("ffmpeg"), ffmpeg.ParseLogLevel)
}

// Start starts the FFmpeg process for a stream.
func (m *streamProcessManager) Start(streamID string) error {
	return m.pool.Start(streamID)
}

// Stop gracefully stops the FFmpeg process for a stream.
func (m *streamProcessManager) Stop(streamID string) error {
	return m.pool.Stop(streamID)
}

// Restart stops and restarts the FFmpeg process with new config.
func (m *streamProcessManager) Restart(streamID string) error {
	return m.pool.Restart(streamID)
}

// GetStatus returns the current state of a stream's process.
func (m *streamProcessManager) GetStatus(streamID string) (*ProcessInfo, error) {
	info := m.pool.GetStatus(streamID)
	return &ProcessInfo{
		StreamID:     info.ID,
		State:        ProcessState(info.State),
		StartedAt:    info.StartedAt,
		RestartCount: info.RestartCount,
		LastError:    info.LastError,
	}, nil
}

// IsRunning checks if a stream's process is currently running.
func (m *streamProcessManager) IsRunning(streamID string) bool {
	return m.pool.IsRunning(streamID)
}

// StartAll starts all enabled streams. Called on daemon startup.
func (m *streamProcessManager) StartAll() error {
	allStreams := m.store.GetAllStreams()

	m.logger.Info("Starting all enabled streams", "total_streams", len(allStreams))

	var startErrors []error

	for streamID := range allStreams {
		if err := m.Start(streamID); err != nil {
			m.logger.Error("Failed to start stream", "stream_id", streamID, "error", err)
			startErrors = append(startErrors, fmt.Errorf("stream %s: %w", streamID, err))
		}
	}

	if len(startErrors) > 0 {
		return errors.Join(startErrors...)
	}

	return nil
}

// StopAll gracefully stops all running processes. Called on shutdown.
func (m *streamProcessManager) StopAll() {
	m.pool.StopAll()
}
