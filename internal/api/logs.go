package api

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/sse"
	"github.com/smazurov/videonode/internal/events"
	"github.com/smazurov/videonode/internal/logging"
)

// registerLogRoutes registers the log streaming SSE endpoint.
func (s *Server) registerLogRoutes() {
	// Register SSE endpoint for log streaming
	sse.Register(s.api, huma.Operation{
		OperationID: "logs-stream",
		Method:      http.MethodGet,
		Path:        "/api/logs/stream",
		Summary:     "Log Stream",
		Description: "Real-time log streaming via Server-Sent Events. Sends historical logs first, then streams new logs.",
		Tags:        []string{"logs"},
		Security:    withAuth(),
		Errors:      []int{401},
	}, func() map[string]any {
		return map[string]any{
			"message": events.LogEntryEvent{},
		}
	}(), func(ctx context.Context, _ *struct{}, send sse.Sender) {
		// First, send all historical logs from the ring buffer
		buffer := logging.GetBuffer()
		if buffer != nil {
			entries := buffer.ReadAll()
			for _, entry := range entries {
				event := events.LogEntryEvent{
					Timestamp:  entry.Timestamp.Format(time.RFC3339Nano),
					Level:      entry.Level,
					Module:     entry.Module,
					Message:    entry.Message,
					Attributes: entry.Attributes,
				}
				if err := send.Data(event); err != nil {
					return
				}
			}
		}

		// Create event channel for this connection
		eventCh := make(chan any, 100) // Larger buffer for logs

		// Subscribe to log events
		unsubscribe := events.SubscribeToChannel[events.LogEntryEvent](s.eventBus, eventCh)
		defer unsubscribe()

		// Stream new log entries as they arrive
		for {
			select {
			case <-ctx.Done():
				return
			case event := <-eventCh:
				if err := send.Data(event); err != nil {
					return
				}
			}
		}
	})
}
