package api

import (
	"context"
	"net/http"
	"sync"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/sse"
	"github.com/smazurov/videonode/internal/obs/exporters"
)

// MetricsEventBroadcaster handles metrics-specific event broadcasting
type MetricsEventBroadcaster struct {
	channels []chan<- interface{}
	mutex    sync.RWMutex
}

var globalMetricsBroadcaster = &MetricsEventBroadcaster{
	channels: make([]chan<- interface{}, 0),
}

// Subscribe adds a channel to receive metrics events
func (mb *MetricsEventBroadcaster) Subscribe(ch chan<- interface{}) {
	mb.mutex.Lock()
	defer mb.mutex.Unlock()
	mb.channels = append(mb.channels, ch)
}

// Unsubscribe removes a channel from receiving metrics events
func (mb *MetricsEventBroadcaster) Unsubscribe(ch chan<- interface{}) {
	mb.mutex.Lock()
	defer mb.mutex.Unlock()
	for i, channel := range mb.channels {
		if channel == ch {
			mb.channels = append(mb.channels[:i], mb.channels[i+1:]...)
			break
		}
	}
}

// Broadcast sends a metrics event to all subscribed channels
func (mb *MetricsEventBroadcaster) Broadcast(event interface{}) {
	mb.mutex.RLock()
	defer mb.mutex.RUnlock()
	for _, ch := range mb.channels {
		select {
		case ch <- event:
		default:
			// Skip if channel is full/blocked
		}
	}
}

// No need for BroadcastMediaMTXMetrics anymore - events come directly from adapter

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
		eventCh := make(chan interface{}, 10)

		// Subscribe to metrics broadcaster
		globalMetricsBroadcaster.Subscribe(eventCh)
		defer globalMetricsBroadcaster.Unsubscribe(eventCh)

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
