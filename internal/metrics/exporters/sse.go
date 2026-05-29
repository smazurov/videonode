package exporters

import (
	"context"
	"sync"
	"time"

	"github.com/smazurov/videonode/internal/events"
	"github.com/smazurov/videonode/internal/metrics"
)

// EntityPublisher emits a per-entity event envelope. Backed by
// *events.Registry so the SSE exporter publishes per-stream metrics on the
// uniform entity envelope the UI stores consume (action=metrics).
type EntityPublisher interface {
	Publish(entityType, action, id string, payload any)
}

// SSEExporter publishes per-stream FFmpeg metrics on the entity envelope.
type SSEExporter struct {
	registry EntityPublisher
	interval time.Duration
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
}

// NewSSEExporter creates a new SSE metrics exporter bound to the entity
// registry it publishes through.
func NewSSEExporter(registry EntityPublisher) *SSEExporter {
	return &SSEExporter{
		registry: registry,
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

		s.registry.Publish("stream", events.ActionMetrics, streamID, map[string]any{
			"fps":              fps,
			"dropped_frames":   dropped,
			"duplicate_frames": dup,
			"bytes_out":        bytesOut,
			"packets_out":      packetsOut,
		})
	}
}
