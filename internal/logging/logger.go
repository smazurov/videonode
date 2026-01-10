package logging

import (
	"log/slog"
	"os"
	"strings"
	"sync"
)

const defaultBufferSize = 1000

var (
	moduleLoggers = make(map[string]*slog.Logger)
	globalConfig  Config
	isInitialized bool
	mutex         sync.RWMutex
	logBuffer     *RingBuffer
	logCallback   LogCallback
)

// Config represents logging configuration.
type Config struct {
	Level   string            `toml:"level"`
	Format  string            `toml:"format"`
	Modules map[string]string `toml:"modules"`
}

// Initialize sets up the logging system.
func Initialize(config Config) {
	mutex.Lock()
	defer mutex.Unlock()

	globalConfig = config
	isInitialized = true

	// Create ring buffer for log history
	logBuffer = NewRingBuffer(defaultBufferSize)

	// Parse global level
	globalLevel := parseLevel(config.Level)
	if globalLevel == nil {
		defaultLevel := slog.LevelInfo
		globalLevel = &defaultLevel
	}

	// Create base handler
	handler := createHandler(config.Format, *globalLevel)

	// Set default logger
	slog.SetDefault(slog.New(handler))

	// Clear existing module loggers since we're reinitializing
	moduleLoggers = make(map[string]*slog.Logger)
}

// GetBuffer returns the log ring buffer for reading historical logs.
func GetBuffer() *RingBuffer {
	mutex.RLock()
	defer mutex.RUnlock()
	return logBuffer
}

// SetLogCallback sets a callback to be called for each new log entry.
// Used for publishing log events to SSE clients.
func SetLogCallback(callback LogCallback) {
	mutex.Lock()
	defer mutex.Unlock()
	logCallback = callback
}

// GetLogger returns a logger for the specified module, creating it if needed.
func GetLogger(module string) *slog.Logger {
	mutex.RLock()
	if logger, exists := moduleLoggers[module]; exists {
		mutex.RUnlock()
		return logger
	}
	mutex.RUnlock()

	// Create logger if it doesn't exist
	mutex.Lock()
	defer mutex.Unlock()

	// Double-check in case another goroutine created it
	if logger, exists := moduleLoggers[module]; exists {
		return logger
	}

	// Determine level for this module
	var moduleLevel slog.Level
	if isInitialized {
		globalLevel := parseLevel(globalConfig.Level)
		if globalLevel != nil {
			moduleLevel = *globalLevel
		} else {
			moduleLevel = slog.LevelInfo
		}

		// Check for module-specific level
		if levelStr, exists := globalConfig.Modules[module]; exists {
			if parsed := parseLevel(levelStr); parsed != nil {
				moduleLevel = *parsed
			}
		}
	} else {
		moduleLevel = slog.LevelInfo
	}

	// Create handler with module-specific level
	var handler slog.Handler
	if isInitialized {
		handler = createHandler(globalConfig.Format, moduleLevel)
	} else {
		handler = createHandler("text", moduleLevel)
	}

	logger := slog.New(handler).With("module", module)
	moduleLoggers[module] = logger
	return logger
}

// createHandler creates a slog handler with the specified format and level.
// Logs to stdout, journal (when available), and ring buffer for SSE streaming.
func createHandler(format string, level slog.Level) slog.Handler {
	opts := &slog.HandlerOptions{Level: level}

	var stdoutHandler slog.Handler
	if format == "json" {
		stdoutHandler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		stdoutHandler = slog.NewTextHandler(os.Stdout, opts)
	}

	journalAvailable := IsJournalAvailable()
	stdoutAvailable := isStdoutAvailable()

	// Create buffer handler if buffer is initialized
	var bufferHandler slog.Handler
	if logBuffer != nil {
		bufferHandler = NewBufferHandler(logBuffer, level, func(entry LogEntry) {
			// Call the callback if set (for SSE publishing)
			if logCallback != nil {
				logCallback(entry)
			}
		})
	}

	// Build handler chain
	var handlers []slog.Handler

	if stdoutAvailable {
		handlers = append(handlers, stdoutHandler)
	}

	if journalAvailable {
		handlers = append(handlers, NewJournalHandler(level))
	}

	if bufferHandler != nil {
		handlers = append(handlers, bufferHandler)
	}

	// Return appropriate handler based on available outputs
	switch len(handlers) {
	case 0:
		return stdoutHandler // Fallback
	case 1:
		return handlers[0]
	default:
		return NewMultiHandler(handlers...)
	}
}

// isStdoutAvailable checks if stdout is connected to a terminal or pipe.
func isStdoutAvailable() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	mode := fi.Mode()
	// Available if terminal, pipe, or regular file (not /dev/null which is ModeDevice)
	return (mode&os.ModeCharDevice) != 0 || (mode&os.ModeNamedPipe) != 0 || mode.IsRegular()
}

// parseLevel converts string level to slog.Level.
func parseLevel(level string) *slog.Level {
	switch strings.ToLower(level) {
	case "debug":
		l := slog.LevelDebug
		return &l
	case "info":
		l := slog.LevelInfo
		return &l
	case "warn", "warning":
		l := slog.LevelWarn
		return &l
	case "error":
		l := slog.LevelError
		return &l
	default:
		return nil
	}
}
