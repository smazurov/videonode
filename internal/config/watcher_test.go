package config

import (
	"fmt"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pelletier/go-toml/v2"
)

type testConfig struct {
	Name  string `toml:"name"`
	Value int    `toml:"value"`
}

func loadTestConfig(path string) (testConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return testConfig{}, err
	}
	var cfg testConfig
	err = toml.Unmarshal(data, &cfg)
	return cfg, err
}

func newTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func TestConfigWatcher_BasicReload(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "config_*.toml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())

	tmpFile.WriteString("name = \"initial\"\nvalue = 1\n")
	tmpFile.Close()

	received := make(chan testConfig, 1)
	watcher := NewConfigWatcher(
		tmpFile.Name(),
		loadTestConfig,
		newTestLogger(),
		WithDebounce[testConfig](50*time.Millisecond),
	)

	watcher.OnReload(func(cfg testConfig) {
		received <- cfg
	})

	if startErr := watcher.Start(); startErr != nil {
		t.Fatal(startErr)
	}
	defer func() {
		if stopErr := watcher.Stop(); stopErr != nil {
			t.Errorf("watcher.Stop failed: %v", stopErr)
		}
	}()

	// Wait for watcher to initialize
	time.Sleep(100 * time.Millisecond)

	// Modify config
	if writeErr := os.WriteFile(tmpFile.Name(), []byte("name = \"updated\"\nvalue = 42\n"), 0o644); writeErr != nil {
		t.Fatal(writeErr)
	}

	select {
	case cfg := <-received:
		if cfg.Name != "updated" || cfg.Value != 42 {
			t.Errorf("got %+v, want name=updated, value=42", cfg)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for config reload")
	}
}

func TestConfigWatcher_FreshConfig(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "config_*.toml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())

	tmpFile.WriteString("value = 1\n")
	tmpFile.Close()

	var loadCount atomic.Int32
	loader := func(path string) (testConfig, error) {
		loadCount.Add(1)
		return loadTestConfig(path)
	}

	received := make(chan testConfig, 10)
	watcher := NewConfigWatcher(
		tmpFile.Name(),
		loader,
		newTestLogger(),
		WithDebounce[testConfig](50*time.Millisecond),
	)

	watcher.OnReload(func(cfg testConfig) {
		received <- cfg
	})

	if startErr := watcher.Start(); startErr != nil {
		t.Fatal(startErr)
	}
	defer func() {
		if stopErr := watcher.Stop(); stopErr != nil {
			t.Errorf("watcher.Stop failed: %v", stopErr)
		}
	}()

	time.Sleep(100 * time.Millisecond)

	// First change
	if writeErr := os.WriteFile(tmpFile.Name(), []byte("value = 10\n"), 0o644); writeErr != nil {
		t.Fatal(writeErr)
	}
	<-received

	// Second change
	time.Sleep(100 * time.Millisecond)
	if writeErr := os.WriteFile(tmpFile.Name(), []byte("value = 20\n"), 0o644); writeErr != nil {
		t.Fatal(writeErr)
	}
	cfg := <-received

	// Verify latest value was loaded
	if cfg.Value != 20 {
		t.Errorf("expected value=20, got %d", cfg.Value)
	}

	// Verify loader was called for each change
	if got := loadCount.Load(); got < 2 {
		t.Errorf("expected at least 2 loads, got %d", got)
	}
}

func TestConfigWatcher_MultipleHandlers(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "config_*.toml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())

	tmpFile.WriteString("name = \"test\"\nvalue = 1\n")
	tmpFile.Close()

	var count atomic.Int32
	var configs []testConfig
	var mu sync.Mutex

	watcher := NewConfigWatcher(
		tmpFile.Name(),
		loadTestConfig,
		newTestLogger(),
		WithDebounce[testConfig](50*time.Millisecond),
	)

	// Register 3 handlers
	for range 3 {
		watcher.OnReload(func(cfg testConfig) {
			count.Add(1)
			mu.Lock()
			configs = append(configs, cfg)
			mu.Unlock()
		})
	}

	if startErr := watcher.Start(); startErr != nil {
		t.Fatal(startErr)
	}
	defer func() {
		if stopErr := watcher.Stop(); stopErr != nil {
			t.Errorf("watcher.Stop failed: %v", stopErr)
		}
	}()

	time.Sleep(100 * time.Millisecond)
	if writeErr := os.WriteFile(tmpFile.Name(), []byte("name = \"new\"\nvalue = 2\n"), 0o644); writeErr != nil {
		t.Fatal(writeErr)
	}

	time.Sleep(200 * time.Millisecond)

	if got := count.Load(); got != 3 {
		t.Errorf("expected 3 handlers called, got %d", got)
	}

	// Verify all handlers received the same config
	mu.Lock()
	defer mu.Unlock()
	for i, cfg := range configs {
		if cfg.Name != "new" || cfg.Value != 2 {
			t.Errorf("handler %d got wrong config: %+v", i, cfg)
		}
	}
}

func TestConfigWatcher_Unsubscribe(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "config_*.toml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())

	tmpFile.WriteString("value = 1\n")
	tmpFile.Close()

	var count1, count2 atomic.Int32
	var lastValue1, lastValue2 atomic.Int32
	watcher := NewConfigWatcher(
		tmpFile.Name(),
		loadTestConfig,
		newTestLogger(),
		WithDebounce[testConfig](50*time.Millisecond),
	)

	watcher.OnReload(func(cfg testConfig) {
		lastValue1.Store(int32(cfg.Value))
		count1.Add(1)
	})
	unsub2 := watcher.OnReload(func(cfg testConfig) {
		lastValue2.Store(int32(cfg.Value))
		count2.Add(1)
	})

	if startErr := watcher.Start(); startErr != nil {
		t.Fatal(startErr)
	}
	defer func() {
		if stopErr := watcher.Stop(); stopErr != nil {
			t.Errorf("watcher.Stop failed: %v", stopErr)
		}
	}()

	// First change - both handlers called
	time.Sleep(100 * time.Millisecond)
	if writeErr := os.WriteFile(tmpFile.Name(), []byte("value = 10\n"), 0o644); writeErr != nil {
		t.Fatal(writeErr)
	}
	time.Sleep(200 * time.Millisecond)

	// Unsubscribe second handler
	unsub2()

	// Second change - only first handler called
	if writeErr := os.WriteFile(tmpFile.Name(), []byte("value = 20\n"), 0o644); writeErr != nil {
		t.Fatal(writeErr)
	}
	time.Sleep(200 * time.Millisecond)

	if got := count1.Load(); got != 2 {
		t.Errorf("handler1: expected 2 calls, got %d", got)
	}
	if got := count2.Load(); got != 1 {
		t.Errorf("handler2: expected 1 call, got %d", got)
	}
	// Verify handlers received correct config values
	if got := lastValue1.Load(); got != 20 {
		t.Errorf("handler1: expected last value 20, got %d", got)
	}
	if got := lastValue2.Load(); got != 10 {
		t.Errorf("handler2: expected last value 10, got %d", got)
	}
}

func TestConfigWatcher_ErrorHandler(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "config_*.toml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())

	tmpFile.WriteString("name = \"valid\"\nvalue = 1\n")
	tmpFile.Close()

	errorReceived := make(chan error, 1)
	configReceived := make(chan testConfig, 1)

	watcher := NewConfigWatcher(
		tmpFile.Name(),
		loadTestConfig,
		newTestLogger(),
		WithDebounce[testConfig](50*time.Millisecond),
		WithErrorHandler[testConfig](func(err error) {
			errorReceived <- err
		}),
	)

	watcher.OnReload(func(cfg testConfig) {
		configReceived <- cfg
	})

	if startErr := watcher.Start(); startErr != nil {
		t.Fatal(startErr)
	}
	defer func() {
		if stopErr := watcher.Stop(); stopErr != nil {
			t.Errorf("watcher.Stop failed: %v", stopErr)
		}
	}()

	// Write invalid TOML
	time.Sleep(100 * time.Millisecond)
	if writeErr := os.WriteFile(tmpFile.Name(), []byte("invalid toml [[["), 0o644); writeErr != nil {
		t.Fatal(writeErr)
	}

	select {
	case <-errorReceived:
		// Expected
	case <-configReceived:
		t.Fatal("config handler should not be called on error")
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for error handler")
	}
}

func TestConfigWatcher_Debounce(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "config_*.toml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())

	tmpFile.WriteString("value = 0\n")
	tmpFile.Close()

	var count atomic.Int32
	var lastValue atomic.Int32

	watcher := NewConfigWatcher(
		tmpFile.Name(),
		loadTestConfig,
		newTestLogger(),
		WithDebounce[testConfig](200*time.Millisecond),
	)

	watcher.OnReload(func(cfg testConfig) {
		count.Add(1)
		lastValue.Store(int32(cfg.Value))
	})

	if startErr := watcher.Start(); startErr != nil {
		t.Fatal(startErr)
	}
	defer func() {
		if stopErr := watcher.Stop(); stopErr != nil {
			t.Errorf("watcher.Stop failed: %v", stopErr)
		}
	}()

	// Rapid changes within debounce window
	time.Sleep(100 * time.Millisecond)
	for i := 1; i <= 5; i++ {
		if writeErr := os.WriteFile(tmpFile.Name(), fmt.Appendf(nil, "value = %d\n", i), 0o644); writeErr != nil {
			t.Fatal(writeErr)
		}
		time.Sleep(50 * time.Millisecond)
	}

	time.Sleep(500 * time.Millisecond)

	if got := count.Load(); got != 1 {
		t.Errorf("expected 1 debounced call, got %d", got)
	}
	if got := lastValue.Load(); got != 5 {
		t.Errorf("expected final value 5, got %d", got)
	}
}

func TestConfigWatcher_ThreadSafety(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "config_*.toml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())

	tmpFile.WriteString("name = \"test\"\n")
	tmpFile.Close()

	watcher := NewConfigWatcher(
		tmpFile.Name(),
		loadTestConfig,
		newTestLogger(),
		WithDebounce[testConfig](10*time.Millisecond),
	)

	if startErr := watcher.Start(); startErr != nil {
		t.Fatal(startErr)
	}
	defer func() {
		if stopErr := watcher.Stop(); stopErr != nil {
			t.Errorf("watcher.Stop failed: %v", stopErr)
		}
	}()

	var wg sync.WaitGroup
	for range 100 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			unsub := watcher.OnReload(func(_ testConfig) {})
			time.Sleep(time.Millisecond)
			unsub()
		}()
	}

	// Trigger some changes while handlers are being added/removed
	for i := range 10 {
		if writeErr := os.WriteFile(tmpFile.Name(), fmt.Appendf(nil, "value = %d\n", i), 0o644); writeErr != nil {
			t.Fatal(writeErr)
		}
		time.Sleep(20 * time.Millisecond)
	}

	wg.Wait()
}

func TestConfigWatcher_Stop(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "config_*.toml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())

	tmpFile.WriteString("value = 1\n")
	tmpFile.Close()

	var count atomic.Int32
	watcher := NewConfigWatcher(
		tmpFile.Name(),
		loadTestConfig,
		newTestLogger(),
		WithDebounce[testConfig](50*time.Millisecond),
	)

	watcher.OnReload(func(_ testConfig) {
		count.Add(1)
	})

	if startErr := watcher.Start(); startErr != nil {
		t.Fatal(startErr)
	}

	time.Sleep(100 * time.Millisecond)

	// Stop watcher
	if stopErr := watcher.Stop(); stopErr != nil {
		t.Fatal(stopErr)
	}

	// Changes after stop should not trigger handler
	if writeErr := os.WriteFile(tmpFile.Name(), []byte("value = 99\n"), 0o644); writeErr != nil {
		t.Fatal(writeErr)
	}
	time.Sleep(200 * time.Millisecond)

	if got := count.Load(); got != 0 {
		t.Errorf("expected 0 calls after stop, got %d", got)
	}
}
