package api

import (
	"log/slog"
	"strconv"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/smazurov/videonode/internal/logging"
)

func HTTPLoggingMiddleware(ctx huma.Context, next func(huma.Context)) {
	start := time.Now()
	logger := logging.GetLogger("http")

	// Extract request information
	method := ctx.Method()
	path := ctx.URL().Path
	query := ctx.URL().RawQuery
	userAgent := ctx.Header("User-Agent")
	remoteAddr := ctx.RemoteAddr()

	// Build base log attributes
	logAttrs := []slog.Attr{
		slog.String("method", method),
		slog.String("path", path),
		slog.String("remote_addr", remoteAddr),
	}

	if query != "" {
		logAttrs = append(logAttrs, slog.String("query", query))
	}

	if userAgent != "" {
		logAttrs = append(logAttrs, slog.String("user_agent", userAgent))
	}

	// Call the next handler
	next(ctx)

	// Calculate duration and get response details
	duration := time.Since(start)
	status := ctx.Status()

	// Add response attributes
	logAttrs = append(logAttrs,
		slog.Int("status", status),
		slog.Duration("duration", duration),
	)

	// Determine log level based on method and status code
	message := "HTTP request completed"
	switch {
	case method == "OPTIONS":
		// CORS preflight requests - DEBUG level
		logger.LogAttrs(ctx.Context(), slog.LevelDebug, message, logAttrs...)
	case status >= 500:
		// Server errors - ERROR level
		logger.LogAttrs(ctx.Context(), slog.LevelError, message, logAttrs...)
	case status >= 400:
		// Client errors - WARN level
		logger.LogAttrs(ctx.Context(), slog.LevelWarn, message, logAttrs...)
	default:
		// Success and redirects - INFO level
		logger.LogAttrs(ctx.Context(), slog.LevelInfo, message, logAttrs...)
	}
}

func formatBytes(bytes int64) string {
	if bytes == 0 {
		return "0"
	}

	const unit = 1024
	if bytes < unit {
		return strconv.FormatInt(bytes, 10) + "B"
	}

	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}

	return strconv.FormatFloat(float64(bytes)/float64(div), 'f', 1, 64) + []string{"B", "KB", "MB", "GB", "TB"}[exp]
}