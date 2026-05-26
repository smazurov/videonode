package api

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/smazurov/videonode/internal/logging"
)

// requestDeadlineMiddleware sets a context deadline on each request so
// handlers that respect ctx.Done() bail out instead of blocking
// indefinitely. Long-lived connections are detected by standard HTTP
// headers and exempted — no path list to maintain:
//   - Accept: text/event-stream → SSE (EventSource always sends this)
//   - Connection: Upgrade → WebSocket / HTTP upgrade
func requestDeadlineMiddleware(next http.Handler, timeout time.Duration) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.Header.Get("Accept"), "text/event-stream") ||
			strings.EqualFold(r.Header.Get("Connection"), "upgrade") {
			next.ServeHTTP(w, r)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		defer cancel()
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// HTTPLoggingMiddleware logs HTTP requests with appropriate log levels based on status codes.
func HTTPLoggingMiddleware(ctx huma.Context, next func(huma.Context)) {
	start := time.Now()
	logger := logging.GetLogger("api")

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
		// Success and redirects - DEBUG level
		logger.LogAttrs(ctx.Context(), slog.LevelDebug, message, logAttrs...)
	}
}
