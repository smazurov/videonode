package logging

import (
	"log/slog"
	"os"
	"strings"
	"sync"
)

var (
	moduleLoggers = make(map[string]*slog.Logger)
	globalConfig  Config
	isInitialized bool
	mutex         sync.RWMutex
)

// Config represents logging configuration
type Config struct {
	Level   string            `toml:"level"`
	Format  string            `toml:"format"`
	Modules map[string]string `toml:"modules"`
}

// Initialize sets up the logging system
func Initialize(config Config) {
	mutex.Lock()
	defer mutex.Unlock()

	globalConfig = config
	isInitialized = true

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

// GetLogger returns a logger for the specified module, creating it if needed
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

// createHandler creates a slog handler with the specified format and level
func createHandler(format string, level slog.Level) slog.Handler {
	isSystemd := os.Getenv("JOURNAL_STREAM") != ""

	opts := &slog.HandlerOptions{Level: level}
	if isSystemd {
		// Remove timestamps for systemd
		opts.ReplaceAttr = func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey {
				return slog.Attr{}
			}
			return a
		}
	}

	if format == "json" {
		return slog.NewJSONHandler(os.Stdout, opts)
	}
	return slog.NewTextHandler(os.Stdout, opts)
}

// parseLevel converts string level to slog.Level
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