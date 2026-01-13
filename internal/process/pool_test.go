package process

import (
	"fmt"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"
)

func poolTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestPoolStartStop(t *testing.T) {
	pool := NewPool(&PoolOptions{
		CommandProvider: func(id string) (string, error) {
			return fmt.Sprintf(`sh -c "trap 'exit 0' INT TERM; echo %s; while :; do sleep 0.1; done"`, id), nil
		},
		Logger: poolTestLogger(),
	})

	if err := pool.Start("test1"); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	if !pool.IsRunning("test1") {
		t.Error("expected process to be running")
	}

	if err := pool.Stop("test1"); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	if pool.IsRunning("test1") {
		t.Error("expected process to not be running")
	}
}

func TestPoolStartAlreadyRunning(t *testing.T) {
	pool := NewPool(&PoolOptions{
		CommandProvider: func(id string) (string, error) {
			return fmt.Sprintf(`sh -c "echo %s; sleep 10"`, id), nil
		},
		Logger: poolTestLogger(),
	})

	if err := pool.Start("test1"); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer pool.StopAll()

	time.Sleep(50 * time.Millisecond)

	err := pool.Start("test1")
	if err == nil {
		t.Error("expected error when starting already running process")
	}
}

func TestPoolGetStatus(t *testing.T) {
	pool := NewPool(&PoolOptions{
		CommandProvider: func(id string) (string, error) {
			return fmt.Sprintf(`sh -c "trap 'exit 0' INT TERM; echo %s; while :; do sleep 0.1; done"`, id), nil
		},
		Logger: poolTestLogger(),
	})

	// Status before start should be idle
	info := pool.GetStatus("test1")
	if info.State != StateIdle {
		t.Errorf("expected StateIdle, got %v", info.State)
	}

	if err := pool.Start("test1"); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer pool.StopAll()

	time.Sleep(50 * time.Millisecond)

	info = pool.GetStatus("test1")
	if info.State != StateRunning {
		t.Errorf("expected StateRunning, got %v", info.State)
	}
	if info.ID != "test1" {
		t.Errorf("expected ID 'test1', got %v", info.ID)
	}
}

func TestPoolRestart(t *testing.T) {
	callCount := 0
	pool := NewPool(&PoolOptions{
		CommandProvider: func(id string) (string, error) {
			callCount++
			return fmt.Sprintf(`sh -c "trap 'exit 0' INT TERM; echo %s; while :; do sleep 0.1; done"`, id), nil
		},
		Logger: poolTestLogger(),
	})

	if err := pool.Start("test1"); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	if err := pool.Restart("test1"); err != nil {
		t.Fatalf("Restart failed: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	if callCount != 2 {
		t.Errorf("expected CommandProvider to be called twice, got %d", callCount)
	}

	pool.StopAll()
}

func TestPoolStopAll(t *testing.T) {
	pool := NewPool(&PoolOptions{
		CommandProvider: func(id string) (string, error) {
			return fmt.Sprintf(`sh -c "trap 'exit 0' INT TERM; echo %s; while :; do sleep 0.1; done"`, id), nil
		},
		Logger: poolTestLogger(),
	})

	_ = pool.Start("test1")
	_ = pool.Start("test2")

	time.Sleep(50 * time.Millisecond)

	if !pool.IsRunning("test1") || !pool.IsRunning("test2") {
		t.Error("expected both processes to be running")
	}

	pool.StopAll()

	if pool.IsRunning("test1") || pool.IsRunning("test2") {
		t.Error("expected both processes to be stopped")
	}
}

func TestPoolStateChangeCallback(t *testing.T) {
	var mu sync.Mutex
	var transitions []struct {
		id       string
		oldState State
		newState State
	}

	pool := NewPool(&PoolOptions{
		CommandProvider: func(id string) (string, error) {
			return fmt.Sprintf("echo %s", id), nil
		},
		OnStateChange: func(id string, oldState, newState State, cbErr error) {
			if cbErr != nil {
				t.Logf("State change error for %s: %v", id, cbErr)
			}
			mu.Lock()
			transitions = append(transitions, struct {
				id       string
				oldState State
				newState State
			}{id, oldState, newState})
			mu.Unlock()
		},
		Logger: poolTestLogger(),
	})

	_ = pool.Start("test1")

	// Wait for process to complete
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if len(transitions) < 2 {
		t.Fatalf("expected at least 2 transitions, got %d", len(transitions))
	}

	// Should have starting -> running transition
	if transitions[0].newState != StateStarting {
		t.Errorf("expected first transition to StateStarting, got %v", transitions[0].newState)
	}
	if transitions[1].newState != StateRunning {
		t.Errorf("expected second transition to StateRunning, got %v", transitions[1].newState)
	}
}

func TestPoolConfigureProcess(t *testing.T) {
	configured := false
	var configuredID string
	pool := NewPool(&PoolOptions{
		CommandProvider: func(id string) (string, error) {
			return fmt.Sprintf("echo %s", id), nil
		},
		ConfigureProcess: func(id string, proc *Process) {
			configuredID = id
			configured = proc != nil
		},
		Logger: poolTestLogger(),
	})

	_ = pool.Start("test1")
	time.Sleep(100 * time.Millisecond)

	if !configured {
		t.Error("expected ConfigureProcess to be called")
	}
	if configuredID != "test1" {
		t.Errorf("expected configuredID 'test1', got %v", configuredID)
	}
}

func TestPoolCommandProviderError(t *testing.T) {
	pool := NewPool(&PoolOptions{
		CommandProvider: func(id string) (string, error) {
			return "", fmt.Errorf("command error for %s", id)
		},
		Logger: poolTestLogger(),
	})

	err := pool.Start("test1")
	if err == nil {
		t.Error("expected error from CommandProvider")
	}
}

func TestPoolProcessCrash(t *testing.T) {
	var mu sync.Mutex
	var lastErr error
	var lastState State
	var lastID string

	pool := NewPool(&PoolOptions{
		CommandProvider: func(id string) (string, error) {
			return fmt.Sprintf("sh -c 'echo %s; exit 42'", id), nil
		},
		OnStateChange: func(id string, oldState, newState State, err error) {
			mu.Lock()
			lastID = id
			lastState = newState
			if err != nil {
				lastErr = err
			}
			mu.Unlock()
			t.Logf("State change: %s %s -> %s (old=%s)", id, oldState, newState, oldState)
		},
		Logger: poolTestLogger(),
	})

	_ = pool.Start("test1")
	time.Sleep(200 * time.Millisecond)

	info := pool.GetStatus("test1")
	if info.State != StateError {
		t.Errorf("expected StateError, got %v", info.State)
	}
	if info.LastError == nil {
		t.Error("expected LastError to be set")
	}

	mu.Lock()
	defer mu.Unlock()
	if lastState != StateError {
		t.Errorf("expected callback to receive StateError, got %v", lastState)
	}
	if lastErr == nil {
		t.Error("expected callback to receive error")
	}
	if lastID != "test1" {
		t.Errorf("expected lastID 'test1', got %v", lastID)
	}
}

func TestPoolStopNotRunning(t *testing.T) {
	pool := NewPool(&PoolOptions{
		CommandProvider: func(id string) (string, error) {
			return fmt.Sprintf("echo %s", id), nil
		},
		Logger: poolTestLogger(),
	})

	// Stop should not error when process is not running
	if err := pool.Stop("nonexistent"); err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestNewPoolPanicsWithoutCommandProvider(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic when CommandProvider is nil")
		}
	}()

	NewPool(nil)
}
