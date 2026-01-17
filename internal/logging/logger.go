package logging

import (
	"log/slog"
	"os"
	"strings"
	"sync"
)

const defaultBufferSize = 1000

// Logger is a duck-typed interface satisfied by *slog.Logger.
// Use this interface instead of *slog.Logger to decouple from the concrete type.
type Logger interface {
	Debug(msg string, args ...any)
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
	Error(msg string, args ...any)
}

var (
	moduleLoggers   = make(map[string]*slog.Logger)
	moduleLevelVars = make(map[string]*slog.LevelVar)
	globalConfig    Config
	globalLevelVar  = &slog.LevelVar{} // default level
	isInitialized   bool
	mutex           sync.RWMutex
	logBuffer       *RingBuffer
	logCallback     LogCallback
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

	// Parse and set global level
	globalLevel := parseLevel(config.Level)
	if globalLevel == nil {
		defaultLevel := slog.LevelInfo
		globalLevel = &defaultLevel
	}
	globalLevelVar.Set(*globalLevel)

	// Update all existing module loggers: set levels and recreate handlers
	// Handlers created before Initialize() lack buffer handler, so we must recreate them
	for module, levelVar := range moduleLevelVars {
		moduleLevel := *globalLevel
		if levelStr, exists := config.Modules[module]; exists {
			if parsed := parseLevel(levelStr); parsed != nil {
				moduleLevel = *parsed
			}
		}
		levelVar.Set(moduleLevel)

		// Recreate handler with full handler chain (including buffer)
		handler := createHandler(config.Format, levelVar)
		moduleLoggers[module] = slog.New(handler).With("module", module)
	}

	// Create base handler for default logger
	handler := createHandler(config.Format, globalLevelVar)

	// Set default logger
	slog.SetDefault(slog.New(handler))
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

	// Create a LevelVar for this module so level can be changed at runtime
	levelVar := &slog.LevelVar{}

	// Determine initial level for this module
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
	levelVar.Set(moduleLevel)

	// Create handler with module-specific LevelVar
	var handler slog.Handler
	if isInitialized {
		handler = createHandler(globalConfig.Format, levelVar)
	} else {
		handler = createHandler("text", levelVar)
	}

	logger := slog.New(handler).With("module", module)
	moduleLoggers[module] = logger
	moduleLevelVars[module] = levelVar
	return logger
}

// createHandler creates a slog handler with the specified format and level.
// Logs to stdout, journal (when available), and ring buffer for SSE streaming.
// Level can be slog.Level or *slog.LevelVar for dynamic level changes.
func createHandler(format string, level slog.Leveler) slog.Handler {
	opts := &slog.HandlerOptions{Level: level}

	var stdoutHandler slog.Handler
	if format == "json" {
		stdoutHandler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		stdoutHandler = slog.NewTextHandler(os.Stdout, opts)
	}

	journalAvailable := IsJournalAvailable()
	stdoutAvailable := isStdoutAvailable()

	// Build handler chain
	var handlers []slog.Handler

	if stdoutAvailable {
		handlers = append(handlers, stdoutHandler)
	}

	if journalAvailable {
		handlers = append(handlers, NewJournalHandler(level))
	}

	// Always add buffer handler - it dynamically checks if buffer is available
	handlers = append(handlers, NewBufferHandler(level))

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

// isStdoutAvailable checks if stdout is connected to a terminal, pipe, socket, or file.
func isStdoutAvailable() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	mode := fi.Mode()
	// Available if terminal, pipe, socket, or regular file (not /dev/null which is ModeDevice)
	return (mode&os.ModeCharDevice) != 0 || (mode&os.ModeNamedPipe) != 0 || (mode&os.ModeSocket) != 0 || mode.IsRegular()
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
