package api

import (
	"context"
	"maps"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/sse"
	"github.com/smazurov/videonode/internal/events"
	"github.com/smazurov/videonode/internal/metrics/exporters"
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
		eventTypes := map[string]any{
			"device-discovery":        events.DeviceDiscoveryEvent{},
			"stream-created":          events.StreamCreatedEvent{},
			"stream-updated":          events.StreamUpdatedEvent{},
			"stream-deleted":          events.StreamDeletedEvent{},
			"stream-state-changed":    events.StreamStateChangedEvent{},
			"stage-state-changed":     events.StageStateChangedEvent{},
			"pipeline-state-changed":  events.PipelineStateChangedEvent{},
			"source-status":           events.SourceStatusEvent{},
			"source-created":          events.SourceCreatedEvent{},
			"source-updated":          events.SourceUpdatedEvent{},
			"source-deleted":          events.SourceDeletedEvent{},
			"composer-created":        events.ComposerCreatedEvent{},
			"composer-updated":        events.ComposerUpdatedEvent{},
			"composer-deleted":        events.ComposerDeletedEvent{},
			"composer-layout-changed": events.ComposerLayoutChangedEvent{},
			"heartbeat":               events.HeartbeatEvent{},
		}

		// Add OBS events for this endpoint
		maps.Copy(eventTypes, exporters.GetEventTypesForEndpoint("events"))

		return eventTypes
	}(), func(ctx context.Context, _ *struct{}, send sse.Sender) {
		eventCh := make(chan any, 10)

		unsubscribers := []func(){
			events.SubscribeToChannel[events.DeviceDiscoveryEvent](s.eventBus, eventCh),
			events.SubscribeToChannel[events.StreamCreatedEvent](s.eventBus, eventCh),
			events.SubscribeToChannel[events.StreamUpdatedEvent](s.eventBus, eventCh),
			events.SubscribeToChannel[events.StreamDeletedEvent](s.eventBus, eventCh),
			events.SubscribeToChannel[events.StreamStateChangedEvent](s.eventBus, eventCh),
			events.SubscribeToChannel[events.StreamMetricsEvent](s.eventBus, eventCh),
			events.SubscribeToChannel[events.SourceStatusEvent](s.eventBus, eventCh),
			events.SubscribeToChannel[events.StageStateChangedEvent](s.eventBus, eventCh),
			events.SubscribeToChannel[events.PipelineStateChangedEvent](s.eventBus, eventCh),
			events.SubscribeToChannel[events.SourceCreatedEvent](s.eventBus, eventCh),
			events.SubscribeToChannel[events.SourceUpdatedEvent](s.eventBus, eventCh),
			events.SubscribeToChannel[events.SourceDeletedEvent](s.eventBus, eventCh),
			events.SubscribeToChannel[events.ComposerCreatedEvent](s.eventBus, eventCh),
			events.SubscribeToChannel[events.ComposerUpdatedEvent](s.eventBus, eventCh),
			events.SubscribeToChannel[events.ComposerDeletedEvent](s.eventBus, eventCh),
			events.SubscribeToChannel[events.ComposerLayoutChangedEvent](s.eventBus, eventCh),
		}
		defer func() {
			for _, unsub := range unsubscribers {
				unsub()
			}
		}()

		// Initial heartbeat flushes HTTP headers so the browser marks the
		// connection as open; the ticker then keeps proxies + idle UIs alive.
		if err := send.Data(events.HeartbeatEvent{Timestamp: time.Now().Format(time.RFC3339)}); err != nil {
			return
		}
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := send.Data(events.HeartbeatEvent{Timestamp: time.Now().Format(time.RFC3339)}); err != nil {
					return
				}
			case event := <-eventCh:
				if err := send.Data(event); err != nil {
					return
				}
			}
		}
	})
}
