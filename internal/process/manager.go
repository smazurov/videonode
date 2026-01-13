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

// LogParser parses a log line and returns the log level and message.
// Used to extract structured log info from process output (ffmpeg, gstreamer, etc.)
type LogParser func(line string) (level, msg string)

type exitReason int

const (
	exitReasonProcessExit exitReason = iota
	exitReasonShutdown
	exitReasonRestart
)

// Manager manages the lifecycle of a subprocess.
type Manager struct {
	streamID        string
	command         string
	commandMu       sync.RWMutex
	cmd             *exec.Cmd
	logger          *slog.Logger
	processLogger   *slog.Logger // logger for process output (nil = use logger)
	logParser       LogParser    // parses process output for log level (nil = no parsing)
	ctx             context.Context
	cancel          context.CancelFunc
	restartChan     chan string // receives new command for restart
	outputHandler   OutputHandler
	gracefulTimeout time.Duration // timeout for graceful shutdown before force kill
	killTimeout     time.Duration // timeout after Kill() before giving up
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
		streamID:        streamID,
		command:         command,
		logger:          logger,
		ctx:             ctx,
		cancel:          cancel,
		restartChan:     make(chan string, 1),
		outputHandler:   handler,
		gracefulTimeout: 5 * time.Second,
		killTimeout:     5 * time.Second,
	}
}

// GetCommand returns the current command string.
func (m *Manager) GetCommand() string {
	m.commandMu.RLock()
	defer m.commandMu.RUnlock()
	return m.command
}

// SetLogParser sets a custom logger and log parser for process output.
// The logger is used for process output (e.g., module="ffmpeg").
// The parser extracts log level from process-specific output formats.
func (m *Manager) SetLogParser(logger *slog.Logger, parser LogParser) {
	m.processLogger = logger
	m.logParser = parser
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

// runningProcess holds channels for monitoring a running subprocess.
type runningProcess struct {
	processDone <-chan error
	outputDone  chan struct{} // receives twice, once per output stream
}

// startProcess parses the command, starts the subprocess, and returns channels for monitoring.
func (m *Manager) startProcess(command string) (*runningProcess, error) {
	args, err := parseCommand(command)
	if err != nil {
		m.logger.Error("Failed to parse command", "error", err)
		return nil, err
	}

	if len(args) == 0 {
		m.logger.Error("Empty command")
		return nil, fmt.Errorf("empty command")
	}

	m.cmd = exec.Command(args[0], args[1:]...)
	m.cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	stdout, err := m.cmd.StdoutPipe()
	if err != nil {
		m.logger.Error("Failed to create stdout pipe", "error", err)
		return nil, err
	}

	stderr, err := m.cmd.StderrPipe()
	if err != nil {
		m.logger.Error("Failed to create stderr pipe", "error", err)
		return nil, err
	}

	if err := m.cmd.Start(); err != nil {
		m.logger.Error("Failed to start process", "error", err, "command", command)
		return nil, err
	}

	m.logger.Info("Process started", "stream_id", m.streamID, "pid", m.cmd.Process.Pid, "command", command)

	// Stream output in separate goroutines
	outputDone := make(chan struct{}, 2)
	go func() {
		m.streamOutput(stdout, "stdout")
		outputDone <- struct{}{}
	}()
	go func() {
		m.streamOutput(stderr, "stderr")
		outputDone <- struct{}{}
	}()

	// Wait for process in goroutine
	processDone := make(chan error, 1)
	go func() {
		processDone <- m.cmd.Wait()
	}()

	return &runningProcess{processDone: processDone, outputDone: outputDone}, nil
}

// waitOutputDone waits for both output streams to complete.
func (m *Manager) waitOutputDone(outputDone <-chan struct{}) {
	<-outputDone
	<-outputDone
}

// exitCodeFromError extracts exit code from process error.
// Returns 0 for nil error, the exit code for ExitError, or 1 for other errors.
func exitCodeFromError(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return 1
}

// handleProcessExit extracts exit code from process error and logs non-ExitError errors.
func (m *Manager) handleProcessExit(processErr error) int {
	exitCode := exitCodeFromError(processErr)
	if processErr != nil && exitCode == 1 {
		m.logger.Error("Process exited with error", "error", processErr)
	}
	return exitCode
}

// Run starts the subprocess and blocks until it exits or receives a signal.
// Returns the exit code of the subprocess.
func (m *Manager) Run() int {
	rp, err := m.startProcess(m.command)
	if err != nil {
		return 1
	}
	defer m.waitOutputDone(rp.outputDone)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigChan)

	select {
	case <-m.ctx.Done():
		m.logger.Info("Context cancelled, shutting down process")
		m.sendStopSignal()
		return m.waitForExit(rp.processDone, m.gracefulTimeout)
	case sig := <-sigChan:
		m.logger.Info("Received shutdown signal", "signal", sig.String())
		m.sendStopSignal()
		return m.waitForExit(rp.processDone, m.gracefulTimeout)
	case processErr := <-rp.processDone:
		exitCode := m.handleProcessExit(processErr)
		m.logger.Info("Process exited", "exit_code", exitCode)
		return exitCode
	}
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

// runOnce runs the process once and returns the exit code and reason for exit.
func (m *Manager) runOnce(sigChan <-chan os.Signal) (int, exitReason) {
	m.commandMu.RLock()
	command := m.command
	m.commandMu.RUnlock()

	rp, err := m.startProcess(command)
	if err != nil {
		return 1, exitReasonProcessExit
	}
	defer m.waitOutputDone(rp.outputDone)

	select {
	case <-m.ctx.Done():
		m.logger.Info("Context cancelled, shutting down process")
		m.sendStopSignal()
		return m.waitForExit(rp.processDone, m.gracefulTimeout), exitReasonShutdown

	case sig := <-sigChan:
		m.logger.Info("Received shutdown signal", "signal", sig.String())
		m.sendStopSignal()
		return m.waitForExit(rp.processDone, m.gracefulTimeout), exitReasonShutdown

	case newCmd := <-m.restartChan:
		m.logger.Info("Received restart request")
		m.sendStopSignal()
		m.commandMu.Lock()
		m.command = newCmd
		m.commandMu.Unlock()
		return m.waitForExit(rp.processDone, m.gracefulTimeout), exitReasonRestart

	case processErr := <-rp.processDone:
		exitCode := m.handleProcessExit(processErr)
		m.logger.Info("Process exited", "exit_code", exitCode)
		return exitCode, exitReasonProcessExit
	}
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
		return exitCodeFromError(err)
	case <-time.After(timeout):
		m.logger.Warn("Graceful shutdown timeout, forcing kill", "timeout", timeout)
		if m.cmd.Process != nil {
			if err := m.cmd.Process.Kill(); err != nil {
				// "os: process already finished" is OK - process exited between timeout and kill
				if !errors.Is(err, os.ErrProcessDone) {
					m.logger.Error("Failed to kill process", "error", err)
				}
			}
		}
		// Wait for process to exit with a secondary timeout to prevent hanging
		select {
		case <-processDone:
			// Process exited
		case <-time.After(m.killTimeout):
			m.logger.Error("Process did not exit after kill signal")
		}
		return 137
	}
}

// streamOutput streams output from the subprocess.
// Uses the configured processLogger (or falls back to manager logger).
// Uses the configured LogParser to extract log levels from process output.
func (m *Manager) streamOutput(reader io.Reader, source string) {
	scanner := bufio.NewScanner(reader)

	// Use process logger if configured, otherwise fall back to manager logger
	logger := m.processLogger
	if logger == nil {
		logger = m.logger
	}

	for scanner.Scan() {
		line := scanner.Text()

		if m.outputHandler != nil {
			m.outputHandler.HandleLine(source, line)
		}

		// Use configured parser or default to info level
		level, msg := "info", line
		if m.logParser != nil {
			level, msg = m.logParser(line)
		}

		switch level {
		case "fatal", "error":
			logger.Error(msg)
		case "warning":
			logger.Warn(msg)
		case "debug", "trace":
			logger.Debug(msg)
		default:
			logger.Info(msg)
		}
	}

	if err := scanner.Err(); err != nil {
		m.logger.Warn("Error reading output", "source", source, "error", err)
	}
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
