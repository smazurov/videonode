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

// EntityPublisher emits a per-entity event envelope. Wired to
// *events.Registry so the SSE exporter can mirror its legacy
// stream-metrics broadcast onto the uniform entity envelope the UI
// stores consume.
type EntityPublisher interface {
	Publish(entityType, action, id string, payload any)
}

// SSEExporter exports FFmpeg stream metrics via Server-Sent Events.
type SSEExporter struct {
	eventBus EventPublisher
	registry EntityPublisher
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

// WithEntityRegistry attaches a registry so emitted metrics also
// publish on the uniform entity envelope (action=metrics). Optional —
// when nil the legacy stream-metrics event is still broadcast.
func (s *SSEExporter) WithEntityRegistry(r EntityPublisher) *SSEExporter {
	s.registry = r
	return s
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
	allMetrics, err := metrics.GetFFmpegMetricsFromRegistry()
	if err != nil {
		return
	}

	egress, _ := metrics.GetStreamEgressMetrics()

	// Collect all stream IDs from both sources.
	streamIDs := make(map[string]struct{})
	for id := range allMetrics {
		streamIDs[id] = struct{}{}
	}
	for id := range egress {
		streamIDs[id] = struct{}{}
	}

	for streamID := range streamIDs {
		ffm := allMetrics[streamID]
		var fps, dropped, dup float64
		if ffm != nil {
			fps = ffm.FPS
			dropped = ffm.DroppedFrames
			dup = ffm.DuplicateFrames
		}

		eg := egress[streamID]
		var bytesOut, packetsOut float64
		if eg != nil {
			bytesOut = eg.BytesOut
			packetsOut = eg.PacketsOut
		}

		metricsEvent := events.StreamMetricsEvent{
			EventType:       "stream_metrics",
			StreamID:        streamID,
			FPS:             strconv.FormatFloat(fps, 'f', 2, 64),
			DroppedFrames:   strconv.FormatFloat(dropped, 'f', 0, 64),
			DuplicateFrames: strconv.FormatFloat(dup, 'f', 0, 64),
			BytesOut:        strconv.FormatFloat(bytesOut, 'f', 0, 64),
			PacketsOut:      strconv.FormatFloat(packetsOut, 'f', 0, 64),
		}
		s.eventBus.Publish(metricsEvent)
		if s.registry != nil {
			s.registry.Publish("stream", events.ActionMetrics, streamID, map[string]any{
				"fps":              fps,
				"dropped_frames":   dropped,
				"duplicate_frames": dup,
				"bytes_out":        bytesOut,
				"packets_out":      packetsOut,
			})
		}
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
