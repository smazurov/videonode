package process

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"
)

// OutputHandler receives output lines from the subprocess.
// Implementations can forward output to NATS, store metrics, etc.
type OutputHandler interface {
	HandleLine(source, line string)
}

// Manager manages the lifecycle of a subprocess (FFmpeg, GStreamer, etc.)
type Manager struct {
	streamID      string
	command       string
	commandMu     sync.RWMutex
	cmd           *exec.Cmd
	logger        *slog.Logger
	ctx           context.Context
	cancel        context.CancelFunc
	restartChan   chan string // receives new command for restart
	outputHandler OutputHandler
}

// NewManager creates a new process manager.
func NewManager(streamID, command string, logger *slog.Logger) *Manager {
	return NewManagerWithOutput(streamID, command, logger, nil)
}

// NewManagerWithOutput creates a new process manager with an output handler.
// The handler receives each line of stdout/stderr from the subprocess.
func NewManagerWithOutput(streamID, command string, logger *slog.Logger, handler OutputHandler) *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{
		streamID:      streamID,
		command:       command,
		logger:        logger,
		ctx:           ctx,
		cancel:        cancel,
		restartChan:   make(chan string, 1),
		outputHandler: handler,
	}
}

// GetCommand returns the current command string.
func (m *Manager) GetCommand() string {
	m.commandMu.RLock()
	defer m.commandMu.RUnlock()
	return m.command
}

// RequestRestart requests a restart with a new command.
// Non-blocking: if a restart is already pending, this is a no-op.
func (m *Manager) RequestRestart(newCommand string) {
	select {
	case m.restartChan <- newCommand:
		m.logger.Info("Restart requested")
	default:
		m.logger.Warn("Restart already pending, ignoring")
	}
}

// Shutdown triggers a graceful shutdown of the manager.
func (m *Manager) Shutdown() {
	m.cancel()
}

// Run starts the subprocess and blocks until it exits or receives a signal
// Returns the exit code of the subprocess.
func (m *Manager) Run() int {
	m.logger.Info("Starting stream process")

	// Parse command string to arguments
	args, err := parseCommand(m.command)
	if err != nil {
		m.logger.Error("Failed to parse command", "error", err)
		return 1
	}

	if len(args) == 0 {
		m.logger.Error("Empty command")
		return 1
	}

	// Create command with context for cancellation
	m.cmd = exec.CommandContext(m.ctx, args[0], args[1:]...)

	// Set process group ID for proper signal handling
	m.cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	// Setup stdout and stderr pipes
	stdout, err := m.cmd.StdoutPipe()
	if err != nil {
		m.logger.Error("Failed to create stdout pipe", "error", err)
		return 1
	}

	stderr, err := m.cmd.StderrPipe()
	if err != nil {
		m.logger.Error("Failed to create stderr pipe", "error", err)
		return 1
	}

	// Start the process
	if startErr := m.cmd.Start(); startErr != nil {
		m.logger.Error("Failed to start process", "error", startErr, "command", m.command)
		return 1
	}

	m.logger.Info("Process started", "pid", m.cmd.Process.Pid)

	// Setup signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Stream output in separate goroutines
	outputDone := make(chan struct{})
	go func() {
		m.streamOutput(stdout, "stdout")
		outputDone <- struct{}{}
	}()
	go func() {
		m.streamOutput(stderr, "stderr")
		outputDone <- struct{}{}
	}()

	// Wait for either signal or process exit
	processDone := make(chan error, 1)
	go func() {
		processDone <- m.cmd.Wait()
	}()

	exitCode := 0
	select {
	case sig := <-sigChan:
		m.logger.Info("Received shutdown signal", "signal", sig.String())
		exitCode = m.Stop(5 * time.Second)
	case processErr := <-processDone:
		// Process exited on its own
		if processErr != nil {
			exitErr := &exec.ExitError{}
			if errors.As(processErr, &exitErr) {
				exitCode = exitErr.ExitCode()
			} else {
				m.logger.Error("Process exited with error", "error", processErr)
				exitCode = 1
			}
		}
		m.logger.Info("Process exited", "exit_code", exitCode)
	}

	// Wait for output streaming to complete
	<-outputDone
	<-outputDone

	m.logger.Info("Process stopped", "exit_code", exitCode)
	return exitCode
}

// RunWithRestart runs the subprocess and handles restart requests.
// It loops, restarting the process when RequestRestart() is called.
// Returns only on shutdown signal or unrecoverable error.
func (m *Manager) RunWithRestart() int {
	// Setup signal handling once for the entire lifecycle
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigChan)

	for {
		exitCode, reason := m.runOnce(sigChan)

		switch reason {
		case exitReasonShutdown:
			m.logger.Info("Shutdown complete", "exit_code", exitCode)
			return exitCode
		case exitReasonRestart:
			m.logger.Info("Restarting process")
			continue
		case exitReasonProcessExit:
			// Process exited unexpectedly - don't restart, let parent handle
			m.logger.Info("Process exited unexpectedly", "exit_code", exitCode)
			return exitCode
		}
	}
}

type exitReason int

const (
	exitReasonProcessExit exitReason = iota
	exitReasonShutdown
	exitReasonRestart
)

// runOnce runs the process once and returns the exit code and reason for exit.
func (m *Manager) runOnce(sigChan <-chan os.Signal) (int, exitReason) {
	m.commandMu.RLock()
	command := m.command
	m.commandMu.RUnlock()

	m.logger.Info("Starting stream process")

	// Parse command string to arguments
	args, err := parseCommand(command)
	if err != nil {
		m.logger.Error("Failed to parse command", "error", err)
		return 1, exitReasonProcessExit
	}

	if len(args) == 0 {
		m.logger.Error("Empty command")
		return 1, exitReasonProcessExit
	}

	// Create command with context for cancellation
	m.cmd = exec.CommandContext(m.ctx, args[0], args[1:]...)

	// Set process group ID for proper signal handling
	m.cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	// Setup stdout and stderr pipes
	stdout, err := m.cmd.StdoutPipe()
	if err != nil {
		m.logger.Error("Failed to create stdout pipe", "error", err)
		return 1, exitReasonProcessExit
	}

	stderr, err := m.cmd.StderrPipe()
	if err != nil {
		m.logger.Error("Failed to create stderr pipe", "error", err)
		return 1, exitReasonProcessExit
	}

	// Start the process
	if startErr := m.cmd.Start(); startErr != nil {
		m.logger.Error("Failed to start process", "error", startErr, "command", command)
		return 1, exitReasonProcessExit
	}

	m.logger.Info("Process started", "pid", m.cmd.Process.Pid)

	// Stream output in separate goroutines
	outputDone := make(chan struct{})
	go func() {
		m.streamOutput(stdout, "stdout")
		outputDone <- struct{}{}
	}()
	go func() {
		m.streamOutput(stderr, "stderr")
		outputDone <- struct{}{}
	}()

	// Wait for process to exit
	processDone := make(chan error, 1)
	go func() {
		processDone <- m.cmd.Wait()
	}()

	var exitCode int
	var reason exitReason

	select {
	case sig := <-sigChan:
		m.logger.Info("Received shutdown signal", "signal", sig.String())
		m.sendStopSignal()
		reason = exitReasonShutdown

	case newCmd := <-m.restartChan:
		m.logger.Info("Received restart request")
		m.sendStopSignal()
		m.commandMu.Lock()
		m.command = newCmd
		m.commandMu.Unlock()
		reason = exitReasonRestart

	case processErr := <-processDone:
		if processErr != nil {
			exitErr := &exec.ExitError{}
			if errors.As(processErr, &exitErr) {
				exitCode = exitErr.ExitCode()
			} else {
				m.logger.Error("Process exited with error", "error", processErr)
				exitCode = 1
			}
		}
		m.logger.Info("Process exited", "exit_code", exitCode)
		reason = exitReasonProcessExit
		// Skip waiting for processDone since we already got it
		<-outputDone
		<-outputDone
		return exitCode, reason
	}

	// Wait for process to actually exit after sending signal
	exitCode = m.waitForExit(processDone, 5*time.Second)

	// Wait for output streaming to complete
	<-outputDone
	<-outputDone

	return exitCode, reason
}

// sendStopSignal sends SIGINT to the subprocess without waiting.
func (m *Manager) sendStopSignal() {
	if m.cmd == nil || m.cmd.Process == nil {
		return
	}
	m.logger.Info("Sending SIGINT to process", "pid", m.cmd.Process.Pid)
	if err := m.cmd.Process.Signal(syscall.SIGINT); err != nil {
		m.logger.Warn("Failed to send SIGINT", "error", err)
	}
}

// waitForExit waits for the process to exit with a timeout, force-killing if needed.
func (m *Manager) waitForExit(processDone <-chan error, timeout time.Duration) int {
	select {
	case err := <-processDone:
		if err != nil {
			exitErr := &exec.ExitError{}
			if errors.As(err, &exitErr) {
				return exitErr.ExitCode()
			}
			return 1
		}
		return 0
	case <-time.After(timeout):
		m.logger.Warn("Graceful shutdown timeout, forcing kill", "timeout", timeout)
		if m.cmd.Process != nil {
			if killErr := m.cmd.Process.Kill(); killErr != nil {
				m.logger.Error("Failed to kill process", "error", killErr)
			}
		}
		// Wait for the kill to complete
		<-processDone
		return 137
	}
}

// Stop gracefully stops the subprocess with a timeout.
func (m *Manager) Stop(timeout time.Duration) int {
	if m.cmd == nil || m.cmd.Process == nil {
		return 0
	}

	// Send SIGINT for graceful shutdown
	m.logger.Info("Sending SIGINT to process", "pid", m.cmd.Process.Pid)
	if sigErr := m.cmd.Process.Signal(syscall.SIGINT); sigErr != nil {
		m.logger.Warn("Failed to send SIGINT", "error", sigErr)
		// Try SIGKILL immediately if SIGINT fails
		if killErr := m.cmd.Process.Kill(); killErr != nil {
			m.logger.Error("Failed to kill process", "error", killErr)
		}
		return 1
	}

	// Wait for graceful shutdown with timeout
	done := make(chan error, 1)
	go func() {
		done <- m.cmd.Wait()
	}()

	select {
	case waitErr := <-done:
		// Process exited gracefully
		if waitErr != nil {
			exitErr := &exec.ExitError{}
			if errors.As(waitErr, &exitErr) {
				return exitErr.ExitCode()
			}
			return 1
		}
		return 0
	case <-time.After(timeout):
		// Timeout - force kill
		m.logger.Warn("Graceful shutdown timeout, forcing kill", "timeout", timeout)
		if killErr := m.cmd.Process.Kill(); killErr != nil {
			m.logger.Error("Failed to kill process", "error", killErr)
		}
		// Wait for kill to complete
		if doneErr := <-done; doneErr != nil {
			exitErr := &exec.ExitError{}
			if errors.As(doneErr, &exitErr) {
				return exitErr.ExitCode()
			}
		}
		return 137 // Standard exit code for SIGKILL
	}
}

// streamOutput streams output from the subprocess with a prefix
// Uses structured logging to send to journal with proper fields.
func (m *Manager) streamOutput(reader io.Reader, source string) {
	scanner := bufio.NewScanner(reader)

	// Create a logger with subprocess context
	subprocessLogger := m.logger.With(
		"stream_id", m.streamID,
		"output_source", source,
		"process_type", "subprocess",
	)

	for scanner.Scan() {
		line := scanner.Text()

		// Call output handler if set
		if m.outputHandler != nil {
			m.outputHandler.HandleLine(source, line)
		}

		// Log with info level - journal fields will be added automatically
		// For non-journal output, include [stream_id] prefix for readability
		if !isJournalLogging() {
			// Standard output format with prefix for text/json logging
			fmt.Printf("[%s] %s\n", m.streamID, line)
		} else {
			// Structured logging to journal - fields are automatically added
			subprocessLogger.Info(line)
		}
	}

	if err := scanner.Err(); err != nil {
		m.logger.Warn("Error reading output", "source", source, "error", err)
	}
}

// isJournalLogging checks if journal logging is active.
func isJournalLogging() bool {
	// Check JOURNAL_STREAM environment variable set by systemd
	return os.Getenv("JOURNAL_STREAM") != ""
}

// parseCommand parses a command string into arguments
// Handles quoted strings and basic escaping.
func parseCommand(command string) ([]string, error) {
	var args []string
	var current strings.Builder
	inQuote := false
	quoteChar := rune(0)

	command = strings.TrimSpace(command)
	runes := []rune(command)

	for i := 0; i < len(runes); i++ {
		r := runes[i]
		switch {
		case r == '"' || r == '\'':
			switch {
			case !inQuote:
				inQuote = true
				quoteChar = r
			case r == quoteChar:
				inQuote = false
				quoteChar = 0
			default:
				current.WriteRune(r)
			}
		case r == ' ' && !inQuote:
			if current.Len() > 0 {
				args = append(args, current.String())
				current.Reset()
			}
		case r == '\\' && i+1 < len(runes):
			// Handle escape sequences
			i++ // Skip the backslash
			current.WriteRune(runes[i])
		default:
			current.WriteRune(r)
		}
	}

	// Add final argument
	if current.Len() > 0 {
		args = append(args, current.String())
	}

	if inQuote {
		return nil, fmt.Errorf("unclosed quote in command")
	}

	return args, nil
}
