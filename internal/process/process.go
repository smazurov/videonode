package process

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/smazurov/videonode/internal/logging"
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

// Process manages the lifecycle of a subprocess.
type Process struct {
	id              string
	command         string
	commandMu       sync.RWMutex
	cmd             *exec.Cmd
	logger          logging.Logger
	processLogger   logging.Logger // logger for process output (nil = use logger)
	logParser       LogParser      // parses process output for log level (nil = no parsing)
	ctx             context.Context
	cancel          context.CancelFunc
	restartChan     chan string // receives new command for restart
	outputHandler   OutputHandler
	gracefulTimeout time.Duration // timeout for graceful shutdown before force kill
	killTimeout     time.Duration // timeout after Kill() before giving up
}

// NewProcess creates a new process.
func NewProcess(id, command string, logger logging.Logger) *Process {
	return NewProcessWithOutput(id, command, logger, nil)
}

// NewProcessWithOutput creates a new process with an output handler.
// The handler receives each line of stdout/stderr from the subprocess.
func NewProcessWithOutput(id, command string, logger logging.Logger, handler OutputHandler) *Process {
	ctx, cancel := context.WithCancel(context.Background())
	return &Process{
		id:              id,
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
func (p *Process) GetCommand() string {
	p.commandMu.RLock()
	defer p.commandMu.RUnlock()
	return p.command
}

// SetLogParser sets a custom logger and log parser for process output.
// The logger is used for process output (e.g., module="ffmpeg").
// The parser extracts log level from process-specific output formats.
func (p *Process) SetLogParser(logger logging.Logger, parser LogParser) {
	p.processLogger = logger
	p.logParser = parser
}

// RequestRestart requests a restart with a new command.
// Non-blocking: if a restart is already pending, this is a no-op.
func (p *Process) RequestRestart(newCommand string) {
	select {
	case p.restartChan <- newCommand:
		p.logger.Info("Restart requested")
	default:
		p.logger.Warn("Restart already pending, ignoring")
	}
}

// Shutdown triggers a graceful shutdown of the process.
func (p *Process) Shutdown() {
	p.cancel()
}

// runningProcess holds channels for monitoring a running subprocess.
type runningProcess struct {
	processDone <-chan error
	outputDone  chan struct{} // receives twice, once per output stream
}

// startProcess parses the command, starts the subprocess, and returns channels for monitoring.
func (p *Process) startProcess(command string) (*runningProcess, error) {
	args, err := parseCommand(command)
	if err != nil {
		p.logger.Error("Failed to parse command", "error", err)
		return nil, err
	}

	if len(args) == 0 {
		p.logger.Error("Empty command")
		return nil, fmt.Errorf("empty command")
	}

	p.cmd = exec.Command(args[0], args[1:]...)
	p.cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	stdout, err := p.cmd.StdoutPipe()
	if err != nil {
		p.logger.Error("Failed to create stdout pipe", "error", err)
		return nil, err
	}

	stderr, err := p.cmd.StderrPipe()
	if err != nil {
		p.logger.Error("Failed to create stderr pipe", "error", err)
		return nil, err
	}

	if err := p.cmd.Start(); err != nil {
		p.logger.Error("Failed to start process", "error", err, "command", command)
		return nil, err
	}

	p.logger.Info("Process started", "id", p.id, "pid", p.cmd.Process.Pid, "command", command)

	// Stream output in separate goroutines
	outputDone := make(chan struct{}, 2)
	go func() {
		p.streamOutput(stdout, "stdout")
		outputDone <- struct{}{}
	}()
	go func() {
		p.streamOutput(stderr, "stderr")
		outputDone <- struct{}{}
	}()

	// Wait for process in goroutine
	processDone := make(chan error, 1)
	go func() {
		processDone <- p.cmd.Wait()
	}()

	return &runningProcess{processDone: processDone, outputDone: outputDone}, nil
}

// waitOutputDone waits for both output streams to complete.
func (p *Process) waitOutputDone(outputDone <-chan struct{}) {
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
func (p *Process) handleProcessExit(processErr error) int {
	exitCode := exitCodeFromError(processErr)
	if processErr != nil && exitCode == 1 {
		p.logger.Error("Process exited with error", "error", processErr)
	}
	return exitCode
}

// Run starts the subprocess and blocks until it exits or receives a signal.
// Returns the exit code of the subprocess.
func (p *Process) Run() int {
	rp, err := p.startProcess(p.command)
	if err != nil {
		return 1
	}
	defer p.waitOutputDone(rp.outputDone)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigChan)

	select {
	case <-p.ctx.Done():
		p.logger.Info("Context cancelled, shutting down process")
		p.sendStopSignal()
		return p.waitForExit(rp.processDone, p.gracefulTimeout)
	case sig := <-sigChan:
		p.logger.Info("Received shutdown signal", "signal", sig.String())
		p.sendStopSignal()
		return p.waitForExit(rp.processDone, p.gracefulTimeout)
	case processErr := <-rp.processDone:
		exitCode := p.handleProcessExit(processErr)
		p.logger.Info("Process exited", "exit_code", exitCode)
		return exitCode
	}
}

// RunWithRestart runs the subprocess and handles restart requests.
// It loops, restarting the process when RequestRestart() is called.
// Returns only on shutdown signal or unrecoverable error.
func (p *Process) RunWithRestart() int {
	// Setup signal handling once for the entire lifecycle
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigChan)

	for {
		exitCode, reason := p.runOnce(sigChan)

		switch reason {
		case exitReasonShutdown:
			p.logger.Info("Shutdown complete", "exit_code", exitCode)
			return exitCode
		case exitReasonRestart:
			p.logger.Info("Restarting process")
			continue
		case exitReasonProcessExit:
			// Process exited unexpectedly - don't restart, let parent handle
			p.logger.Info("Process exited unexpectedly", "exit_code", exitCode)
			return exitCode
		}
	}
}

// runOnce runs the process once and returns the exit code and reason for exit.
func (p *Process) runOnce(sigChan <-chan os.Signal) (int, exitReason) {
	p.commandMu.RLock()
	command := p.command
	p.commandMu.RUnlock()

	rp, err := p.startProcess(command)
	if err != nil {
		return 1, exitReasonProcessExit
	}
	defer p.waitOutputDone(rp.outputDone)

	select {
	case <-p.ctx.Done():
		p.logger.Info("Context cancelled, shutting down process")
		p.sendStopSignal()
		return p.waitForExit(rp.processDone, p.gracefulTimeout), exitReasonShutdown

	case sig := <-sigChan:
		p.logger.Info("Received shutdown signal", "signal", sig.String())
		p.sendStopSignal()
		return p.waitForExit(rp.processDone, p.gracefulTimeout), exitReasonShutdown

	case newCmd := <-p.restartChan:
		p.logger.Info("Received restart request")
		p.sendStopSignal()
		p.commandMu.Lock()
		p.command = newCmd
		p.commandMu.Unlock()
		return p.waitForExit(rp.processDone, p.gracefulTimeout), exitReasonRestart

	case processErr := <-rp.processDone:
		exitCode := p.handleProcessExit(processErr)
		p.logger.Info("Process exited", "exit_code", exitCode)
		return exitCode, exitReasonProcessExit
	}
}

// sendStopSignal sends SIGINT to the subprocess without waiting.
func (p *Process) sendStopSignal() {
	if p.cmd == nil || p.cmd.Process == nil {
		return
	}
	p.logger.Info("Sending SIGINT to process", "pid", p.cmd.Process.Pid)
	if err := p.cmd.Process.Signal(syscall.SIGINT); err != nil {
		p.logger.Warn("Failed to send SIGINT", "error", err)
	}
}

// waitForExit waits for the process to exit with a timeout, force-killing if needed.
func (p *Process) waitForExit(processDone <-chan error, timeout time.Duration) int {
	select {
	case err := <-processDone:
		return exitCodeFromError(err)
	case <-time.After(timeout):
		p.logger.Warn("Graceful shutdown timeout, forcing kill", "timeout", timeout)
		if p.cmd.Process != nil {
			if err := p.cmd.Process.Kill(); err != nil {
				// "os: process already finished" is OK - process exited between timeout and kill
				if !errors.Is(err, os.ErrProcessDone) {
					p.logger.Error("Failed to kill process", "error", err)
				}
			}
		}
		// Wait for process to exit with a secondary timeout to prevent hanging
		select {
		case <-processDone:
			// Process exited
		case <-time.After(p.killTimeout):
			p.logger.Error("Process did not exit after kill signal")
		}
		return 137
	}
}

// streamOutput streams output from the subprocess.
// Uses the configured processLogger (or falls back to default logger).
// Uses the configured LogParser to extract log levels from process output.
func (p *Process) streamOutput(reader io.Reader, source string) {
	scanner := bufio.NewScanner(reader)

	// Use process logger if configured, otherwise fall back to default logger
	logger := p.processLogger
	if logger == nil {
		logger = p.logger
	}

	for scanner.Scan() {
		line := scanner.Text()

		if p.outputHandler != nil {
			p.outputHandler.HandleLine(source, line)
		}

		// Use configured parser or default to info level
		level, msg := "info", line
		if p.logParser != nil {
			level, msg = p.logParser(line)
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
		p.logger.Warn("Error reading output", "source", source, "error", err)
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
