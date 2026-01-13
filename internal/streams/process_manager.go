package streams

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
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

// managedProcess tracks a running FFmpeg process.
type managedProcess struct {
	manager      *process.Manager
	streamID     string
	state        ProcessState
	startedAt    time.Time
	restartCount int
	lastError    error
	cancel       context.CancelFunc
	done         chan struct{}
}

// streamProcessManager implements StreamProcessManager.
type streamProcessManager struct {
	store          Store
	processor      *processor
	eventBus       *events.Bus
	processes      map[string]*managedProcess
	processesMutex sync.RWMutex
	logger         *slog.Logger
	ctx            context.Context
	cancel         context.CancelFunc
	wg             sync.WaitGroup
}

// ProcessManagerOptions contains options for creating a StreamProcessManager.
type ProcessManagerOptions struct {
	Store     Store
	Processor *processor
	EventBus  *events.Bus
}

// NewStreamProcessManager creates a new StreamProcessManager.
func NewStreamProcessManager(opts *ProcessManagerOptions) StreamProcessManager {
	ctx, cancel := context.WithCancel(context.Background())

	return &streamProcessManager{
		store:     opts.Store,
		processor: opts.Processor,
		eventBus:  opts.EventBus,
		processes: make(map[string]*managedProcess),
		logger:    logging.GetLogger("process_manager"),
		ctx:       ctx,
		cancel:    cancel,
	}
}

// Start starts the FFmpeg process for a stream.
func (m *streamProcessManager) Start(streamID string) error {
	m.processesMutex.Lock()
	defer m.processesMutex.Unlock()

	// Check if already running
	if proc, exists := m.processes[streamID]; exists {
		if proc.state == ProcessStateRunning || proc.state == ProcessStateStarting {
			return fmt.Errorf("stream %s already running", streamID)
		}
	}

	// Generate FFmpeg command
	processed, err := m.processor.processStream(streamID)
	if err != nil {
		return fmt.Errorf("failed to generate FFmpeg command: %w", err)
	}

	return m.startProcess(streamID, processed.FFmpegCommand)
}

// startProcess starts a process with the given command (must hold lock).
func (m *streamProcessManager) startProcess(streamID string, command string) error {
	ctx, cancel := context.WithCancel(m.ctx)

	proc := &managedProcess{
		streamID:  streamID,
		state:     ProcessStateStarting,
		startedAt: time.Now(),
		cancel:    cancel,
		done:      make(chan struct{}),
	}

	// Create process manager with ffmpeg log parsing
	proc.manager = process.NewManager(streamID, command, m.logger)
	proc.manager.SetLogParser(logging.GetLogger("ffmpeg"), ffmpeg.ParseLogLevel)

	m.processes[streamID] = proc

	// Start process in goroutine
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		defer close(proc.done)

		m.runProcess(ctx, proc)
	}()

	return nil
}

// runProcess runs the process and handles state transitions.
func (m *streamProcessManager) runProcess(ctx context.Context, proc *managedProcess) {
	// Update state to running
	m.processesMutex.Lock()
	proc.state = ProcessStateRunning
	m.processesMutex.Unlock()

	// Emit process started event
	if m.eventBus != nil {
		m.eventBus.Publish(events.StreamStateChangedEvent{
			StreamID:  proc.streamID,
			Enabled:   true,
			Timestamp: time.Now().Format(time.RFC3339),
		})
	}

	// Run process - blocks until exit
	exitCode := proc.manager.Run()

	// Update state based on exit
	m.processesMutex.Lock()
	switch {
	case ctx.Err() != nil:
		// Context cancelled - graceful shutdown
		proc.state = ProcessStateIdle
	case exitCode != 0:
		// Process crashed
		proc.state = ProcessStateError
		proc.lastError = fmt.Errorf("process exited with code %d", exitCode)
		m.logger.Error("Stream process crashed", "stream_id", proc.streamID, "exit_code", exitCode)
	default:
		proc.state = ProcessStateIdle
	}
	m.processesMutex.Unlock()

	m.logger.Info("Stream process stopped", "stream_id", proc.streamID, "exit_code", exitCode)
}

// Stop gracefully stops the FFmpeg process for a stream.
func (m *streamProcessManager) Stop(streamID string) error {
	m.processesMutex.Lock()
	proc, exists := m.processes[streamID]
	if !exists {
		m.processesMutex.Unlock()
		return nil // Not running, nothing to stop
	}

	if proc.state != ProcessStateRunning && proc.state != ProcessStateStarting {
		m.processesMutex.Unlock()
		return nil // Already stopped
	}

	proc.state = ProcessStateStopping
	m.processesMutex.Unlock()

	m.logger.Info("Stopping stream process", "stream_id", streamID)

	// Cancel context and shutdown manager
	proc.cancel()
	proc.manager.Shutdown()

	// Wait for process to exit with timeout
	select {
	case <-proc.done:
		// Process exited
	case <-time.After(10 * time.Second):
		m.logger.Warn("Timeout waiting for process to stop", "stream_id", streamID)
	}

	// Remove from map
	m.processesMutex.Lock()
	delete(m.processes, streamID)
	m.processesMutex.Unlock()

	return nil
}

// Restart stops and restarts the FFmpeg process with new config.
func (m *streamProcessManager) Restart(streamID string) error {
	m.logger.Info("Restarting stream process", "stream_id", streamID)

	// Stop existing process (Stop() waits for cleanup to complete)
	if err := m.Stop(streamID); err != nil {
		return fmt.Errorf("failed to stop process: %w", err)
	}

	// Start with new config
	return m.Start(streamID)
}

// GetStatus returns the current state of a stream's process.
func (m *streamProcessManager) GetStatus(streamID string) (*ProcessInfo, error) {
	m.processesMutex.RLock()
	defer m.processesMutex.RUnlock()

	proc, exists := m.processes[streamID]
	if !exists {
		return &ProcessInfo{
			StreamID: streamID,
			State:    ProcessStateIdle,
		}, nil
	}

	info := &ProcessInfo{
		StreamID:     streamID,
		State:        proc.state,
		StartedAt:    proc.startedAt,
		RestartCount: proc.restartCount,
		LastError:    proc.lastError,
	}

	return info, nil
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
	m.logger.Info("Stopping all stream processes")

	// Cancel parent context to signal all processes
	m.cancel()

	// Get list of running processes
	m.processesMutex.RLock()
	streamIDs := make([]string, 0, len(m.processes))
	for streamID := range m.processes {
		streamIDs = append(streamIDs, streamID)
	}
	m.processesMutex.RUnlock()

	// Stop each process
	for _, streamID := range streamIDs {
		if err := m.Stop(streamID); err != nil {
			m.logger.Error("Failed to stop stream", "stream_id", streamID, "error", err)
		}
	}

	// Wait for all goroutines to finish
	m.wg.Wait()

	m.logger.Info("All stream processes stopped")
}

// IsRunning checks if a stream's process is currently running.
func (m *streamProcessManager) IsRunning(streamID string) bool {
	m.processesMutex.RLock()
	defer m.processesMutex.RUnlock()

	proc, exists := m.processes[streamID]
	if !exists {
		return false
	}

	return proc.state == ProcessStateRunning
}
