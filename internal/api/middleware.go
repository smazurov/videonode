package api

import (
	"log/slog"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/smazurov/videonode/internal/logging"
)

// HTTPLoggingMiddleware logs HTTP requests with appropriate log levels based on status codes.
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
