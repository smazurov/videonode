package process

import (
	"io"
	"log/slog"
	"testing"
	"time"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// newTestManager creates a Manager with short timeouts for testing.
func newTestManager(command string) *Process {
	m := NewProcess("test", command, testLogger())
	m.gracefulTimeout = 100 * time.Millisecond
	m.killTimeout = 100 * time.Millisecond
	return m
}

// runAsync runs the manager's Run method in a goroutine and returns exit code channel.
func runAsync(m *Process) <-chan int {
	done := make(chan int, 1)
	go func() {
		done <- m.Run()
	}()
	return done
}

// runWithRestartAsync runs RunWithRestart in a goroutine and returns exit code channel.
func runWithRestartAsync(m *Process) <-chan int {
	done := make(chan int, 1)
	go func() {
		done <- m.RunWithRestart()
	}()
	return done
}

// waitForExit waits for exit code with timeout, fails test on timeout.
func waitForExit(t *testing.T, done <-chan int, timeout time.Duration) int {
	t.Helper()
	select {
	case exitCode := <-done:
		return exitCode
	case <-time.After(timeout):
		t.Fatal("timeout waiting for process to exit")
		return -1
	}
}

func TestGracefulShutdown(t *testing.T) {
	// Process that handles SIGINT
	m := newTestManager(`sh -c "trap 'exit 0' INT TERM; while :; do sleep 0.1; done"`)
	m.gracefulTimeout = 500 * time.Millisecond

	done := runAsync(m)
	time.Sleep(100 * time.Millisecond)
	m.Shutdown()

	if exitCode := waitForExit(t, done, 1*time.Second); exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}
}

func TestForceKillOnTimeout(t *testing.T) {
	// Process that ignores SIGINT
	m := newTestManager(`sh -c "trap '' INT; sleep 10"`)
	m.gracefulTimeout = 50 * time.Millisecond
	m.killTimeout = 50 * time.Millisecond

	done := runAsync(m)
	time.Sleep(50 * time.Millisecond)
	m.Shutdown()

	// Process was killed, expect 137 (128 + 9 for SIGKILL)
	if exitCode := waitForExit(t, done, 500*time.Millisecond); exitCode != 137 {
		t.Errorf("expected exit code 137, got %d", exitCode)
	}
}

func TestContextCancellation(t *testing.T) {
	m := newTestManager("sleep 10")

	done := runAsync(m)
	time.Sleep(50 * time.Millisecond)

	start := time.Now()
	m.Shutdown()
	waitForExit(t, done, 500*time.Millisecond)

	if elapsed := time.Since(start); elapsed > 300*time.Millisecond {
		t.Errorf("shutdown took too long: %v", elapsed)
	}
}

func TestProcessAlreadyExited(t *testing.T) {
	m := newTestManager("true")

	done := runAsync(m)
	if exitCode := waitForExit(t, done, 500*time.Millisecond); exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}

	// Shutdown after process has already exited - should not panic
	m.Shutdown()
}

func TestGetCommand(t *testing.T) {
	m := newTestManager("echo hello")
	if got := m.GetCommand(); got != "echo hello" {
		t.Errorf("GetCommand() = %q, want %q", got, "echo hello")
	}
}

func TestRequestRestart(t *testing.T) {
	m := newTestManager("sleep 10")

	done := runWithRestartAsync(m)
	time.Sleep(100 * time.Millisecond)

	m.RequestRestart("echo restarted")
	time.Sleep(100 * time.Millisecond)

	if got := m.GetCommand(); got != "echo restarted" {
		t.Errorf("GetCommand() after restart = %q, want %q", got, "echo restarted")
	}

	m.Shutdown()
	waitForExit(t, done, 1*time.Second)
}

func TestRunWithRestart(t *testing.T) {
	m := newTestManager("true")

	done := runWithRestartAsync(m)
	if exitCode := waitForExit(t, done, 500*time.Millisecond); exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}
}

func TestRunWithRestartShutdown(t *testing.T) {
	m := newTestManager(`sh -c "trap 'exit 0' INT TERM; while :; do sleep 0.1; done"`)
	m.gracefulTimeout = 500 * time.Millisecond

	done := runWithRestartAsync(m)
	time.Sleep(100 * time.Millisecond)
	m.Shutdown()

	if exitCode := waitForExit(t, done, 1*time.Second); exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}
}

func TestRequestRestartAlreadyPending(t *testing.T) {
	m := newTestManager("sleep 10")

	m.RequestRestart("echo first")
	m.RequestRestart("echo second") // Should be ignored

	if got := <-m.restartChan; got != "echo first" {
		t.Errorf("expected 'echo first', got %q", got)
	}
}

func TestRunWithInvalidCommand(t *testing.T) {
	m := newTestManager(`echo "unclosed`)
	if exitCode := m.Run(); exitCode != 1 {
		t.Errorf("expected exit code 1 for parse error, got %d", exitCode)
	}
}

func TestRunWithEmptyCommand(t *testing.T) {
	m := newTestManager("")
	if exitCode := m.Run(); exitCode != 1 {
		t.Errorf("expected exit code 1 for empty command, got %d", exitCode)
	}
}

func TestRunWithRestartInvalidCommand(t *testing.T) {
	m := newTestManager(`echo "unclosed`)
	if exitCode := m.RunWithRestart(); exitCode != 1 {
		t.Errorf("expected exit code 1 for parse error, got %d", exitCode)
	}
}

func TestProcessExitWithError(t *testing.T) {
	m := newTestManager("sh -c 'exit 42'")
	if exitCode := m.Run(); exitCode != 42 {
		t.Errorf("expected exit code 42, got %d", exitCode)
	}
}

func TestRunWithNonExistentCommand(t *testing.T) {
	m := newTestManager("/nonexistent/command/that/does/not/exist")
	if exitCode := m.Run(); exitCode != 1 {
		t.Errorf("expected exit code 1 for start error, got %d", exitCode)
	}
}

func TestShutdownBeforeStart(t *testing.T) {
	m := newTestManager("sleep 10")
	m.Shutdown() // Should not panic
	t.Log("Shutdown before start completed without panic")
}

func TestParseCommandWithEscapes(t *testing.T) {
	args, err := parseCommand(`echo hello\ world`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(args) != 2 || args[1] != "hello world" {
		t.Errorf("expected ['echo', 'hello world'], got %v", args)
	}
}

func TestSendStopSignalAfterExit(t *testing.T) {
	m := newTestManager("true")
	if exitCode := m.Run(); exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}
	m.sendStopSignal() // Should not panic, process already exited
}

func TestStreamOutputLogLevels(t *testing.T) {
	cmd := `echo "[error] error message" && echo "[warning] warn message" && echo "[debug] debug message" && echo "[fatal] fatal message" && echo "plain message"`
	m := newTestManager("sh -c '" + cmd + "'")
	if exitCode := m.Run(); exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}
}

func TestOutputHandler(t *testing.T) {
	var lines []string
	handler := &testOutputHandler{lines: &lines}

	m := NewProcessWithOutput("test", `sh -c "echo line1; echo line2"`, testLogger(), handler)
	m.gracefulTimeout = 100 * time.Millisecond
	m.killTimeout = 100 * time.Millisecond

	if exitCode := m.Run(); exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}
	if len(lines) < 2 {
		t.Errorf("expected at least 2 lines, got %d: %v", len(lines), lines)
	}
}

type testOutputHandler struct {
	lines *[]string
}

func (h *testOutputHandler) HandleLine(_, line string) {
	*h.lines = append(*h.lines, line)
}
