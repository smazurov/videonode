package api

import (
	"context"
	"maps"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/sse"
	"github.com/smazurov/videonode/internal/events"
	"github.com/smazurov/videonode/internal/obs/exporters"
)

// registerSSERoutes registers the native Huma SSE endpoint.
func (s *Server) registerSSERoutes() {
	// Register SSE endpoint with event type mapping
	sse.Register(s.api, huma.Operation{
		OperationID: "events-stream",
		Method:      http.MethodGet,
		Path:        "/api/events",
		Summary:     "Server-Sent Events Stream",
		Description: "Real-time event stream for capture results, device changes, and system status",
		Tags:        []string{"events"},
		Security:    withAuth(),
		Errors:      []int{401},
	}, func() map[string]any {
		// Application events
		eventTypes := map[string]any{
			"capture-success":      events.CaptureSuccessEvent{},
			"capture-error":        events.CaptureErrorEvent{},
			"device-discovery":     events.DeviceDiscoveryEvent{},
			"stream-created":       events.StreamCreatedEvent{},
			"stream-updated":       events.StreamUpdatedEvent{},
			"stream-deleted":       events.StreamDeletedEvent{},
			"stream-state-changed": events.StreamStateChangedEvent{},
		}

		// Add OBS events for this endpoint
		maps.Copy(eventTypes, exporters.GetEventTypesForEndpoint("events"))

		return eventTypes
	}(), func(ctx context.Context, _ *struct{}, send sse.Sender) {
		// Create event channel for this connection
		eventCh := make(chan any, 10)

		// Subscribe to all event types using event bus
		unsubscribers := []func(){
			events.SubscribeToChannel[events.CaptureSuccessEvent](s.eventBus, eventCh),
			events.SubscribeToChannel[events.CaptureErrorEvent](s.eventBus, eventCh),
			events.SubscribeToChannel[events.DeviceDiscoveryEvent](s.eventBus, eventCh),
			events.SubscribeToChannel[events.StreamCreatedEvent](s.eventBus, eventCh),
			events.SubscribeToChannel[events.StreamUpdatedEvent](s.eventBus, eventCh),
			events.SubscribeToChannel[events.StreamDeletedEvent](s.eventBus, eventCh),
			events.SubscribeToChannel[events.StreamStateChangedEvent](s.eventBus, eventCh),
			events.SubscribeToChannel[events.OBSAlertEvent](s.eventBus, eventCh),
			events.SubscribeToChannel[events.StreamMetricsEvent](s.eventBus, eventCh),
		}
		defer func() {
			for _, unsub := range unsubscribers {
				unsub()
			}
		}()

		// Send initial connection confirmation
		if err := send.Data(events.CaptureSuccessEvent{
			DevicePath: "system",
			Message:    "SSE connection established",
			ImageData:  "",
			Timestamp:  time.Now().Format(time.RFC3339),
		}); err != nil {
			return
		}

		// Keep connection alive and forward events
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
