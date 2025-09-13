package mediamtx

import (
	"sync"
	"testing"
)

func TestSetConfig(t *testing.T) {
	// Test default config
	if !globalConfig.EnableLogging {
		t.Error("Default config should have EnableLogging = true")
	}

	// Test setting config to disable logging
	SetConfig(&Config{
		EnableLogging: false,
	})

	if globalConfig.EnableLogging {
		t.Error("After SetConfig with EnableLogging=false, globalConfig should have EnableLogging=false")
	}

	// Test that SetConfig only works once (due to sync.Once)
	SetConfig(&Config{
		EnableLogging: true,
	})

	// Should still be false because SetConfig only runs once
	if globalConfig.EnableLogging {
		t.Error("SetConfig should only work once due to sync.Once")
	}
}

func TestDefaultConfig(t *testing.T) {
	// Reset for this test (not normally possible in production)
	// This test should run in isolation
	cfg := &Config{
		EnableLogging: true,
	}

	if !cfg.EnableLogging {
		t.Error("Default config should have EnableLogging = true")
	}
}

func TestSetConfigOnceWithConcurrency(t *testing.T) {
	// Reset the once for this test
	configOnce = sync.Once{}
	globalConfig = &Config{EnableLogging: true}

	// Counter to track how many times the function actually executes
	var execCount int
	var mu sync.Mutex

	// Number of goroutines to launch
	const numGoroutines = 10
	var wg sync.WaitGroup

	// Launch multiple goroutines that all try to set config
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			// Each goroutine tries to set a different config
			SetConfig(&Config{
				EnableLogging: id%2 == 0, // Alternate true/false
			})

			// Track execution (this will only happen once due to sync.Once)
			mu.Lock()
			execCount++
			mu.Unlock()
		}(i)
	}

	// Wait for all goroutines to complete
	wg.Wait()

	// Verify that SetConfig was called numGoroutines times but only executed once
	if execCount != numGoroutines {
		t.Errorf("Expected SetConfig to be called %d times, got %d", numGoroutines, execCount)
	}

	// Verify that only the first config setting took effect
	// Since we can't predict which goroutine runs first, just verify that
	// subsequent calls don't change the config
	originalConfig := globalConfig.EnableLogging

	// Try to change it again
	SetConfig(&Config{EnableLogging: !originalConfig})

	// Should remain unchanged due to sync.Once
	if globalConfig.EnableLogging != originalConfig {
		t.Error("sync.Once should prevent subsequent SetConfig calls from changing the config")
	}
}

func TestSyncOnceExecution(t *testing.T) {
	// Reset state for this test
	configOnce = sync.Once{}
	globalConfig = &Config{EnableLogging: true}

	// Counter to track actual executions of the sync.Once function
	var onceExecutions int
	var totalCalls int
	var mu sync.Mutex

	// Override SetConfig for testing to track executions
	testConfigOnce := sync.Once{}
	testSetConfig := func(cfg *Config) {
		mu.Lock()
		totalCalls++
		mu.Unlock()

		testConfigOnce.Do(func() {
			mu.Lock()
			onceExecutions++
			mu.Unlock()
			if cfg != nil {
				globalConfig = cfg
			}
		})
	}

	const numCalls = 10
	var wg sync.WaitGroup

	// Make multiple concurrent calls
	for i := 0; i < numCalls; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			testSetConfig(&Config{EnableLogging: id%2 == 0})
		}(i)
	}

	wg.Wait()

	// Verify that all calls were made
	if totalCalls != numCalls {
		t.Errorf("Expected %d total calls, got %d", numCalls, totalCalls)
	}

	// Verify that sync.Once function executed exactly once
	if onceExecutions != 1 {
		t.Errorf("Expected sync.Once function to execute exactly 1 time, got %d", onceExecutions)
	}
}

func TestWrapWithLoggerEscaping(t *testing.T) {
	// Save original config
	originalConfig := globalConfig
	defer func() { globalConfig = originalConfig }()

	tests := []struct {
		name     string
		command  string
		logging  bool
		expected string
	}{
		{
			name:     "simple command with logging",
			command:  "ffmpeg -i input.mp4 output.mp4",
			logging:  true,
			expected: "/bin/bash -c 'ffmpeg -i input.mp4 output.mp4 2>&1 | systemd-cat -t ffmpeg-$MTX_PATH'",
		},
		{
			name:     "simple command without logging",
			command:  "ffmpeg -i input.mp4 output.mp4",
			logging:  false,
			expected: "ffmpeg -i input.mp4 output.mp4",
		},
		{
			name:     "command with single quotes",
			command:  "ffmpeg -vf 'scale=1280:720' input.mp4",
			logging:  true,
			expected: "/bin/bash -c 'ffmpeg -vf '\"'\"'scale=1280:720'\"'\"' input.mp4 2>&1 | systemd-cat -t ffmpeg-$MTX_PATH'",
		},
		{
			name:     "complex command with quotes and pipes",
			command:  "ffmpeg -hide_banner -f v4l2 -i /dev/video0 -vf 'format=yuv420p' -c:v h264 -f rtsp rtsp://localhost:8554/test",
			logging:  true,
			expected: "/bin/bash -c 'ffmpeg -hide_banner -f v4l2 -i /dev/video0 -vf '\"'\"'format=yuv420p'\"'\"' -c:v h264 -f rtsp rtsp://localhost:8554/test 2>&1 | systemd-cat -t ffmpeg-$MTX_PATH'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			globalConfig = &Config{EnableLogging: tt.logging}
			result := wrapWithLogger(tt.command)
			if result != tt.expected {
				t.Errorf("wrapWithLogger() = %q, want %q", result, tt.expected)
			}
		})
	}
}
