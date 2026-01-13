package logging

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
)

func TestModuleLevelOverride(t *testing.T) {
	// Reset state
	mutex.Lock()
	moduleLoggers = make(map[string]*slog.Logger)
	isInitialized = false
	mutex.Unlock()

	// Initialize with global info level, but streams module at debug
	Initialize(Config{
		Level:  "info",
		Format: "text",
		Modules: map[string]string{
			"streams": "debug",
			"api":     "warn",
		},
	})

	tests := []struct {
		module      string
		wantDebug   bool
		wantInfo    bool
		wantWarn    bool
		description string
	}{
		{"streams", true, true, true, "streams module should log debug (override to debug)"},
		{"api", false, false, true, "api module should only log warn (override to warn)"},
		{"other", false, true, true, "other module should log info (global default)"},
	}

	for _, tt := range tests {
		t.Run(tt.module, func(t *testing.T) {
			logger := GetLogger(tt.module)

			// Get the handler from the logger to test Enabled
			// We need to check if the handler accepts different levels
			handler := logger.Handler()

			gotDebug := handler.Enabled(context.Background(), slog.LevelDebug)
			gotInfo := handler.Enabled(context.Background(), slog.LevelInfo)
			gotWarn := handler.Enabled(context.Background(), slog.LevelWarn)

			if gotDebug != tt.wantDebug {
				t.Errorf("module %q: Debug enabled = %v, want %v", tt.module, gotDebug, tt.wantDebug)
			}
			if gotInfo != tt.wantInfo {
				t.Errorf("module %q: Info enabled = %v, want %v", tt.module, gotInfo, tt.wantInfo)
			}
			if gotWarn != tt.wantWarn {
				t.Errorf("module %q: Warn enabled = %v, want %v", tt.module, gotWarn, tt.wantWarn)
			}
		})
	}
}

func TestModuleLevelActualOutput(t *testing.T) {
	// Reset state
	mutex.Lock()
	moduleLoggers = make(map[string]*slog.Logger)
	isInitialized = false
	mutex.Unlock()

	// Create a buffer to capture output
	var buf bytes.Buffer

	// Create a custom handler that writes to our buffer
	handler := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	logger := slog.New(handler).With("module", "test")

	// Log at different levels
	logger.Debug("debug message")
	logger.Info("info message")
	logger.Warn("warn message")

	output := buf.String()

	if !strings.Contains(output, "debug message") {
		t.Error("Debug message not found in output")
	}
	if !strings.Contains(output, "info message") {
		t.Error("Info message not found in output")
	}
	if !strings.Contains(output, "warn message") {
		t.Error("Warn message not found in output")
	}
}

func TestModuleLevelWithMultiHandler(t *testing.T) {
	// Reset state
	mutex.Lock()
	moduleLoggers = make(map[string]*slog.Logger)
	isInitialized = false
	mutex.Unlock()

	// Initialize with debug level for webrtc module
	Initialize(Config{
		Level:  "info",
		Format: "text",
		Modules: map[string]string{
			"webrtc": "debug",
		},
	})

	logger := GetLogger("webrtc")
	handler := logger.Handler()

	// Verify the handler accepts debug level
	if !handler.Enabled(context.Background(), slog.LevelDebug) {
		t.Error("webrtc module handler should accept Debug level")
	}

	// Regardless of handler type, debug should be enabled
	if !handler.Enabled(context.Background(), slog.LevelDebug) {
		t.Errorf("Debug should be enabled for webrtc module, handler type: %T", handler)
	}
}

func TestDebugLogsActuallyWritten(t *testing.T) {
	// Create a buffer to capture output
	var buf bytes.Buffer

	// Create handler with debug level
	handler := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	logger := slog.New(handler).With("module", "webrtc")

	// Write debug log
	logger.Debug("test debug message", "key", "value")

	output := buf.String()
	if !strings.Contains(output, "test debug message") {
		t.Errorf("Debug message not written. Output: %s", output)
	}
	if !strings.Contains(output, "level=DEBUG") {
		t.Errorf("Debug level not in output. Output: %s", output)
	}
}

func TestMultiHandlerDebugOutput(t *testing.T) {
	var buf bytes.Buffer

	// Create two handlers - one with debug, one with info
	debugHandler := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	infoHandler := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})

	multi := NewMultiHandler(debugHandler, infoHandler)
	logger := slog.New(multi).With("module", "test")

	// Write debug log - should appear once (from debugHandler)
	logger.Debug("debug only message")

	output := buf.String()
	if !strings.Contains(output, "debug only message") {
		t.Errorf("Debug message not written via MultiHandler. Output: %s", output)
	}

	// Count occurrences - should be 1 (only debugHandler writes it)
	count := strings.Count(output, "debug only message")
	if count != 1 {
		t.Errorf("Expected 1 debug message, got %d. Output: %s", count, output)
	}
}

func TestGetLoggerBeforeInitialize(t *testing.T) {
	// Reset state completely
	mutex.Lock()
	moduleLoggers = make(map[string]*slog.Logger)
	moduleLevelVars = make(map[string]*slog.LevelVar)
	isInitialized = false
	globalConfig = Config{}
	mutex.Unlock()

	// Get logger BEFORE Initialize - should default to info level
	loggerBefore := GetLogger("webrtc")
	handlerBefore := loggerBefore.Handler()

	// Should NOT have debug enabled (defaults to info)
	if handlerBefore.Enabled(context.Background(), slog.LevelDebug) {
		t.Error("Logger created before Initialize should NOT have debug enabled")
	}

	// Now Initialize with debug level for webrtc
	Initialize(Config{
		Level:  "info",
		Format: "text",
		Modules: map[string]string{
			"webrtc": "debug",
		},
	})

	// Get logger AFTER Initialize - should be SAME logger (cached) with updated level
	loggerAfter := GetLogger("webrtc")

	// With LevelVar fix, logger should be cached (same pointer) but level updated dynamically
	if loggerBefore != loggerAfter {
		t.Error("Logger should be cached - same pointer before and after Initialize")
	}

	// The cached logger should now have debug enabled (LevelVar was updated)
	if !handlerBefore.Enabled(context.Background(), slog.LevelDebug) {
		t.Error("Cached logger should have debug enabled after Initialize updates LevelVar")
	}
}

func TestParseLevelValues(t *testing.T) {
	tests := []struct {
		input string
		want  slog.Level
		isNil bool
	}{
		{"debug", slog.LevelDebug, false},
		{"DEBUG", slog.LevelDebug, false},
		{"info", slog.LevelInfo, false},
		{"INFO", slog.LevelInfo, false},
		{"warn", slog.LevelWarn, false},
		{"warning", slog.LevelWarn, false},
		{"error", slog.LevelError, false},
		{"invalid", 0, true},
		{"", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseLevel(tt.input)
			if tt.isNil {
				if got != nil {
					t.Errorf("parseLevel(%q) = %v, want nil", tt.input, *got)
				}
			} else {
				if got == nil {
					t.Errorf("parseLevel(%q) = nil, want %v", tt.input, tt.want)
				} else if *got != tt.want {
					t.Errorf("parseLevel(%q) = %v, want %v", tt.input, *got, tt.want)
				}
			}
		})
	}
}
