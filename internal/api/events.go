package api

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/sse"
	"github.com/smazurov/videonode/internal/events"
)

// registerSSERoutes registers the native Huma SSE endpoint.
func (s *Server) registerSSERoutes() {
	// Must precede sse.Register: the single "entity" event's schema is a
	// discriminated union built by EntityEvent.Schema from these variants.
	registerEntityVariants()
	// Register SSE endpoint with event type mapping. The wire carries the
	// uniform entity envelope (all per-entity lifecycle/status/metrics/
	// consumers events) plus a few genuinely-global events and a keep-alive
	// heartbeat. The UI discriminates entity events on the `type` tag.
	sse.Register(s.api, huma.Operation{
		OperationID: "events-stream",
		Method:      http.MethodGet,
		Path:        "/api/events",
		Summary:     "Server-Sent Events Stream",
		Description: "Real-time event stream for entity lifecycle/status/metrics/consumers, device changes, pipeline state, and supervised-process stats",
		Tags:        []string{"events"},
		Security:    withAuth(),
		Errors:      []int{401},
	}, map[string]any{
		"entity":                 events.EntityEvent{},
		"device-discovery":       events.DeviceDiscoveryEvent{},
		"pipeline-state-changed": events.PipelineStateChangedEvent{},
		"processes":              events.ProcessesEvent{},
		"process-removed":        events.ProcessRemovedEvent{},
		"heartbeat":              events.HeartbeatEvent{},
	}, func(ctx context.Context, _ *sseInput, send sse.Sender) {
		eventCh := make(chan any, 10)

		unsubscribers := []func(){
			events.SubscribeToChannel[events.EntityEvent](s.eventBus, eventCh),
			events.SubscribeToChannel[events.DeviceDiscoveryEvent](s.eventBus, eventCh),
			events.SubscribeToChannel[events.PipelineStateChangedEvent](s.eventBus, eventCh),
			// The pipeline publishes ProcessesEvent with the internal pool
			// vocabulary ("producer:" ids); normalize each row to the
			// user-facing "source:" shape — the same edge translation the
			// REST /api/processes handler does — before it hits the wire.
			events.Subscribe(s.eventBus, func(e events.ProcessesEvent) {
				select {
				case eventCh <- normalizeProcessesEvent(e):
				default:
				}
			}),
			events.Subscribe(s.eventBus, func(e events.ProcessRemovedEvent) {
				e.ID = normalizeProcessID(e.ID)
				select {
				case eventCh <- e:
				default:
				}
			}),
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
