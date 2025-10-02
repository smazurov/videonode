package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/sse"
	"github.com/smazurov/videonode/internal/events"
	"github.com/smazurov/videonode/internal/obs/exporters"
)

// registerMetricsRoutes registers the metrics SSE endpoint
func (s *Server) registerMetricsRoutes() {
	// Register metrics SSE endpoint
	sse.Register(s.api, huma.Operation{
		OperationID: "metrics-stream",
		Method:      http.MethodGet,
		Path:        "/api/metrics",
		Summary:     "Metrics Server-Sent Events Stream",
		Description: "Real-time metrics stream for MediaMTX streaming metrics",
		Tags:        []string{"metrics"},
		Security:    withAuth(),
		Errors:      []int{401},
	}, func() map[string]any {
		// Get metrics event types from OBS exporters
		return exporters.GetEventTypesForEndpoint("metrics")
	}(), func(ctx context.Context, input *struct{}, send sse.Sender) {
		// Create event channel for this connection
		eventCh := make(chan any, 10)

		// Subscribe to metrics events using event bus
		unsubscribe := events.SubscribeToChannel[events.MediaMTXMetricsEvent](s.eventBus, eventCh)
		defer unsubscribe()

		// Keep connection alive and forward metrics events
		for {
			select {
			case <-ctx.Done():
				return
			case event := <-eventCh:
				// Send event using Huma's SSE sender with error handling
				if err := send.Data(event); err != nil {
					// Connection failed, clean up and exit
					return
				}
			}
		}
	})
}
