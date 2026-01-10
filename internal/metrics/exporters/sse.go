package exporters

import (
	"context"
	"strconv"
	"sync"
	"time"

	"github.com/smazurov/videonode/internal/events"
	"github.com/smazurov/videonode/internal/metrics"
)

// EventPublisher interface for publishing events.
type EventPublisher interface {
	Publish(ev events.Event)
}

// SSEExporter exports FFmpeg stream metrics via Server-Sent Events.
type SSEExporter struct {
	eventBus EventPublisher
	interval time.Duration
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
}

// NewSSEExporter creates a new SSE exporter.
func NewSSEExporter(eventBus EventPublisher) *SSEExporter {
	return &SSEExporter{
		eventBus: eventBus,
		interval: 1 * time.Second,
	}
}

// Start begins the SSE export loop.
func (s *SSEExporter) Start(ctx context.Context) {
	s.ctx, s.cancel = context.WithCancel(ctx)
	s.wg.Add(1)
	go s.run()
}

// Stop stops the SSE exporter and waits for the goroutine to finish.
func (s *SSEExporter) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
	s.wg.Wait()
}

func (s *SSEExporter) run() {
	defer s.wg.Done()
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.publishMetrics()
		}
	}
}

func (s *SSEExporter) publishMetrics() {
	allMetrics := metrics.GetAllFFmpegMetrics()
	for streamID, m := range allMetrics {
		s.eventBus.Publish(events.StreamMetricsEvent{
			EventType:       "stream_metrics",
			StreamID:        streamID,
			FPS:             strconv.FormatFloat(m.FPS, 'f', 2, 64),
			DroppedFrames:   strconv.FormatFloat(m.DroppedFrames, 'f', 0, 64),
			DuplicateFrames: strconv.FormatFloat(m.DuplicateFrames, 'f', 0, 64),
		})
	}
}

// GetEventTypes returns event types for SSE endpoint registration.
func GetEventTypes() map[string]any {
	return map[string]any{
		"stream-metrics": events.StreamMetricsEvent{},
	}
}

// GetEventTypesForEndpoint returns event types for a specific SSE endpoint.
func GetEventTypesForEndpoint(endpoint string) map[string]any {
	if endpoint == "events" {
		return map[string]any{
			"stream-metrics": events.StreamMetricsEvent{},
		}
	}
	return map[string]any{}
}

// GetEventRoutes returns the routing configuration for events.
func GetEventRoutes() map[string]string {
	return map[string]string{
		"stream-metrics": "events",
	}
}
