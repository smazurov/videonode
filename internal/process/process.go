package process

import (
	"bufio"
	"bytes"
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
	"sync/atomic"
	"syscall"
	"time"

	"github.com/smazurov/videonode/internal/logging"
)

// OutputHandler receives output lines from the subprocess.
type OutputHandler interface {
	HandleLine(source, line string)
}

// LogParser extracts (level, msg, attrs) from a process output line.
type LogParser func(line string) (level, msg string, attrs []slog.Attr)

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
	processLogger   logging.Logger
	logParser       LogParser
	ctx             context.Context
	cancel          context.CancelFunc
	restartChan     chan string
	outputHandler   OutputHandler
	gracefulTimeout time.Duration
	killTimeout     time.Duration
	pid             atomic.Int32
}

// NewProcess creates a new process.
func NewProcess(id, command string, logger logging.Logger) *Process {
	return NewProcessWithOutput(id, command, logger, nil)
}

// NewProcessWithOutput creates a new process; handler receives each line of stdout/stderr.
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

// PID returns the OS process ID, or 0 if the process hasn't started.
// Safe to call concurrently — uses an atomic.
func (p *Process) PID() int { return int(p.pid.Load()) }

// GetCommand returns the current command string.
func (p *Process) GetCommand() string {
	p.commandMu.RLock()
	defer p.commandMu.RUnlock()
	return p.command
}

// SetLogParser sets the logger and parser used for child process output.
func (p *Process) SetLogParser(logger logging.Logger, parser LogParser) {
	p.processLogger = logger
	p.logParser = parser
}

// SetLogger overrides the logger used for supervisor lifecycle events. Callers
// route it to a per-entity module logger so these lines land on the owning
// entity's log page instead of the orchestration module.
func (p *Process) SetLogger(logger logging.Logger) {
	p.logger = logger
}

// Logger returns the supervisor logger so the pool can attribute its own
// lifecycle lines to the same per-entity module.
func (p *Process) Logger() logging.Logger {
	return p.logger
}

// RequestRestart requests a restart with a new command; no-op when one is pending.
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

type runningProcess struct {
	processDone <-chan error
	outputDone  chan struct{} // closed once both output streams are drained
}

// startProcess parses the command, starts the subprocess, and returns channels for monitoring.
func (p *Process) startProcess(command string) (*runningProcess, error) {
	args, err := parseCommand(command)
	if err != nil {
		p.logger.Error("Failed to parse command", logging.KeyError, err)
		return nil, err
	}

	if len(args) == 0 {
		p.logger.Error("Empty command")
		return nil, fmt.Errorf("empty command")
	}

	p.cmd = exec.Command(args[0], args[1:]...)
	// Pdeathsig kills child on parent crash; otherwise canvas restart leaks ffmpegs holding v4l2.
	p.cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid:   true,
		Pdeathsig: syscall.SIGKILL,
	}

	stdout, err := p.cmd.StdoutPipe()
	if err != nil {
		p.logger.Error("Failed to create stdout pipe", logging.KeyError, err)
		return nil, err
	}

	stderr, err := p.cmd.StderrPipe()
	if err != nil {
		p.logger.Error("Failed to create stderr pipe", logging.KeyError, err)
		return nil, err
	}

	if err := p.cmd.Start(); err != nil {
		p.logger.Error("Failed to start process", logging.KeyError, err, logging.KeyCommand, command)
		return nil, err
	}

	p.pid.Store(int32(p.cmd.Process.Pid))

	p.logger.Info("Process started", logging.KeyPoolID, p.id, logging.KeyPID, p.cmd.Process.Pid, logging.KeyCommand, command)

	outputDone := make(chan struct{})
	var streams sync.WaitGroup
	streams.Add(2)
	go func() {
		defer streams.Done()
		p.streamOutput(stdout, "stdout")
	}()
	go func() {
		defer streams.Done()
		p.streamOutput(stderr, "stderr")
	}()

	processDone := make(chan error, 1)
	go func() {
		// cmd.Wait closes the pipes, discarding unread output; drain first.
		streams.Wait()
		close(outputDone)
		processDone <- p.cmd.Wait()
	}()

	return &runningProcess{processDone: processDone, outputDone: outputDone}, nil
}

// waitOutputDone waits for both output streams to complete.
func (p *Process) waitOutputDone(outputDone <-chan struct{}) {
	<-outputDone
}

// exitCodeFromError returns 0 for nil, the exit code for ExitError, else 1.
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

// handleProcessExit extracts the exit code and logs non-ExitError failures.
func (p *Process) handleProcessExit(processErr error) int {
	exitCode := exitCodeFromError(processErr)
	if processErr != nil && exitCode == 1 {
		p.logger.Debug("Process exited with error", logging.KeyError, processErr)
	}
	return exitCode
}

// Run starts the subprocess and blocks until it exits or receives a signal.
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
		p.logger.Debug("Context cancelled, shutting down process")
		p.sendStopSignal()
		return p.waitForExit(rp.processDone, p.gracefulTimeout)
	case sig := <-sigChan:
		p.logger.Debug("Received shutdown signal", logging.KeySignal, sig.String())
		p.sendStopSignal()
		return p.waitForExit(rp.processDone, p.gracefulTimeout)
	case processErr := <-rp.processDone:
		exitCode := p.handleProcessExit(processErr)
		p.logger.Debug("Process exited", logging.KeyExitCode, exitCode)
		return exitCode
	}
}

// RunWithRestart loops the subprocess, honoring RequestRestart; returns on shutdown.
func (p *Process) RunWithRestart() int {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigChan)

	for {
		exitCode, reason := p.runOnce(sigChan)

		switch reason {
		case exitReasonShutdown:
			p.logger.Info("Shutdown complete", logging.KeyExitCode, exitCode)
			return exitCode
		case exitReasonRestart:
			p.logger.Info("Restarting process")
			continue
		case exitReasonProcessExit:
			// Don't restart unexpected exits; let the parent decide.
			p.logger.Info("Process exited unexpectedly", logging.KeyExitCode, exitCode)
			return exitCode
		}
	}
}

// runOnce runs the process once and returns (exitCode, reason).
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
		p.logger.Info("Received shutdown signal", logging.KeySignal, sig.String())
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
		p.logger.Info("Process exited", logging.KeyExitCode, exitCode)
		return exitCode, exitReasonProcessExit
	}
}

// sendStopSignal sends SIGINT to the subprocess's process group without
// waiting. Targeting the group (negative pid) is required for `sh -c "A | B"`
// encoder pipelines so the piped children die with the shell — otherwise
// they reparent to init and keep holding the gRPC socket / RTSP producer
// slot, producing the black-canvas symptom on restart. The cmd has
// Setpgid:true, so pid == pgid.
func (p *Process) sendStopSignal() {
	if p.cmd == nil || p.cmd.Process == nil {
		return
	}
	pid := p.cmd.Process.Pid
	p.logger.Info("Sending SIGINT to process group", logging.KeyPID, pid)
	if err := syscall.Kill(-pid, syscall.SIGINT); err != nil {
		p.logger.Warn("Failed to send SIGINT to group", logging.KeyError, err)
	}
}

// waitForExit waits for exit with a timeout, force-killing if needed.
func (p *Process) waitForExit(processDone <-chan error, timeout time.Duration) int {
	select {
	case err := <-processDone:
		return exitCodeFromError(err)
	case <-time.After(timeout):
		p.logger.Warn("Graceful shutdown timeout, forcing kill", logging.KeyTimeout, timeout)
		if p.cmd.Process != nil {
			// Kill the whole process group; see sendStopSignal for why.
			if err := syscall.Kill(-p.cmd.Process.Pid, syscall.SIGKILL); err != nil {
				if !errors.Is(err, syscall.ESRCH) {
					p.logger.Error("Failed to kill process group", logging.KeyError, err)
				}
			}
		}
		select {
		case <-processDone:
		case <-time.After(p.killTimeout):
			p.logger.Error("Process did not exit after kill signal")
		}
		return 137
	}
}

// scanCRLines is bufio.ScanLines with bare \r accepted as a terminator (\r\n
// counts once), so ffmpeg-style carriage-return progress updates emit as
// individual lines instead of accumulating until the next newline.
func scanCRLines(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}
	i := bytes.IndexAny(data, "\r\n")
	if i < 0 {
		if atEOF {
			return len(data), data, nil
		}
		return 0, nil, nil
	}
	if data[i] == '\n' {
		return i + 1, data[:i], nil
	}
	// \r at the end of the buffer: wait for more data to see if \n follows.
	if i == len(data)-1 && !atEOF {
		return 0, nil, nil
	}
	advance = i + 1
	if advance < len(data) && data[advance] == '\n' {
		advance++
	}
	return advance, data[:i], nil
}

// streamOutput pipes child output through processLogger (fallback: logger) and logParser.
func (p *Process) streamOutput(reader io.Reader, source string) {
	scanner := bufio.NewScanner(reader)
	scanner.Split(scanCRLines)

	logger := p.processLogger
	if logger == nil {
		logger = p.logger
	}

	for scanner.Scan() {
		line := scanner.Text()

		if p.outputHandler != nil {
			p.outputHandler.HandleLine(source, line)
		}

		level, msg := "info", line
		var attrs []slog.Attr
		if p.logParser != nil {
			level, msg, attrs = p.logParser(line)
		}

		args := make([]any, len(attrs))
		for i, a := range attrs {
			args[i] = a
		}

		switch level {
		case "fatal", "error":
			logger.Error(msg, args...)
		case "warning":
			logger.Warn(msg, args...)
		case "debug", "trace":
			logger.Debug(msg, args...)
		default:
			logger.Info(msg, args...)
		}
	}

	if err := scanner.Err(); err != nil {
		p.logger.Warn("Error reading output", logging.KeyPipe, source, logging.KeyError, err)
	}
}

// parseCommand splits a command string into argv, honoring quoted segments and \-escapes.
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
			i++
			current.WriteRune(runes[i])
		default:
			current.WriteRune(r)
		}
	}

	if current.Len() > 0 {
		args = append(args, current.String())
	}

	if inQuote {
		return nil, fmt.Errorf("unclosed quote in command")
	}

	return args, nil
}
