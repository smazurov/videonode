// Package logging provides structured logging with per-module log level configuration.
//
// # Overview
//
// The logging system uses Go's slog package with automatic output routing:
//   - Logs to systemd journal when available (Linux systems with journald)
//   - Logs to stdout when a terminal, pipe, or file is connected
//   - Logs to both when both are available
//
// # Usage
//
// Initialize the logging system once at startup:
//
//	logging.Initialize(logging.Config{
//		Level:  "info",      // Global log level: debug, info, warn, error
//		Format: "text",      // Output format: text or json
//		Modules: map[string]string{
//			"streams": "debug",  // Per-module overrides
//			"api":     "warn",
//		},
//	})
//
// Get a logger for your module:
//
//	logger := logging.GetLogger("mymodule")
//	logger.Info("Starting up", "port", 8080)
//	logger.Debug("Details", "config", cfg)
//	logger.Warn("Something unusual", "error", err)
//	logger.Error("Failed", "error", err)
//
// Add contextual attributes:
//
//	logger := logging.GetLogger("streams").With("stream_id", id)
//	logger.Info("Stream started")  // Includes stream_id in all logs
//
// # Log Levels
//
//	debug - Verbose debugging information
//	info  - General operational messages
//	warn  - Warning conditions
//	error - Error conditions
//
// # Output Destinations
//
// The system automatically detects available outputs:
//
//	Journal available + stdout available → MultiHandler (both)
//	Journal available only              → JournalHandler
//	Stdout available only               → TextHandler or JSONHandler
//
// Journal availability is checked via [github.com/coreos/go-systemd/v22/journal.Enabled].
//
// # Viewing Logs
//
// When running as a systemd service or on a system with journald:
//
//	journalctl -t videonode              # All videonode logs
//	journalctl -t videonode -f           # Follow live
//	journalctl -t videonode --since "5m" # Last 5 minutes
//	journalctl -t videonode -p err       # Errors only
//
// Filter by structured fields:
//
//	journalctl -t videonode MODULE=streams
//	journalctl -t videonode STREAM_ID=test
//
// # Configuration
//
// Log levels can be set globally or per-module. Module-specific levels
// override the global level for that module only.
//
// Example TOML configuration:
//
//	[logging]
//	level = "info"
//	format = "text"
//
//	[logging.modules]
//	streams = "debug"
//	api = "warn"
//	obs = "error"
package logging
