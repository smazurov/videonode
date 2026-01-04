package exporters

import (
	"fmt"
	"testing"
	"time"

	"github.com/smazurov/videonode/internal/events"
	"github.com/smazurov/videonode/internal/obs"
)

// TestMemoryLeak_NoTTL demonstrates that metrics accumulate in memory forever.
func TestMemoryLeak_NoTTL(t *testing.T) {
	exporter := NewPromExporter()

	// Create 1000 unique metrics
	for i := range 1000 {
		metric := &obs.MetricPoint{
			Name:          "test_metric",
			Value:         float64(i),
			LabelsMap:     obs.Labels{"unique_id": fmt.Sprintf("metric_%d", i)},
			TimestampUnix: time.Now().Add(-time.Duration(i) * time.Second),
		}
		if err := exporter.Export([]obs.DataPoint{metric}); err != nil {
			t.Fatalf("Export failed: %v", err)
		}
	}

	exporter.ForceFlush()

	// With stable labels fix, these should be deduplicated to 1 metric
	actualCount := len(exporter.collector.metrics)
	switch actualCount {
	case 1000:
		t.Errorf("MEMORY LEAK: All 1000 unique metrics kept in memory - no cleanup mechanism")
	case 1:
		t.Log("FIXED: Stable labels working - metrics are deduplicated")
	default:
		t.Logf("Got %d metrics (unexpected)", actualCount)
	}
}

// TestRingBufferWorks shows Prometheus exporter now uses ring buffer and deduplication.
func TestRingBufferWorks(t *testing.T) {
	// Store uses ring buffer - only keeps recent points
	storeConfig := obs.StoreConfig{
		MaxPointsPerSeries: 10, // Small ring buffer
		MaxSeries:          100,
	}

	store := obs.NewStore(storeConfig)

	// Add 50 points to same series
	for i := range 50 {
		point := &obs.MetricPoint{
			Name:          "test_series",
			Value:         float64(i),
			LabelsMap:     obs.Labels{"series": "test"},
			TimestampUnix: time.Now().Add(time.Duration(i) * time.Second),
		}
		if err := store.Add(point); err != nil {
			t.Fatalf("store.Add failed: %v", err)
		}
	}

	// Store should only keep last 10 points
	result, _ := store.Query(obs.QueryOptions{
		DataType: obs.DataTypeMetric,
		Name:     "test_series",
		Labels:   obs.Labels{"series": "test"},
	})
	if len(result.Points) != 10 {
		t.Errorf("Store ring buffer failed: expected 10 points, got %d", len(result.Points))
	}

	// Prometheus exporter now has stable labels and deduplication
	exporter := NewPromExporter()

	for i := range 50 {
		metric := &obs.MetricPoint{
			Name:      "prom_test",
			Value:     float64(i),
			LabelsMap: obs.Labels{"stream_id": "test"}, // Use stable label
		}
		if err := exporter.Export([]obs.DataPoint{metric}); err != nil {
			t.Fatalf("Export failed: %v", err)
		}
	}

	exporter.ForceFlush()

	// Prometheus now deduplicates to 1 metric (fixed behavior)
	if len(exporter.collector.metrics) != 1 {
		t.Errorf("Expected 1 deduplicated metric, got %d", len(exporter.collector.metrics))
	}
}

// TestFFmpegMemoryGrowth simulates real FFmpeg scenario.
func TestFFmpegMemoryGrowth(t *testing.T) {
	exporter := NewPromExporter()

	// Simulate 1 hour of changing duplicate frame counts
	for second := range 3600 {
		metric := &obs.MetricPoint{
			Name:  "ffmpeg_duplicate_frames_total",
			Value: float64(100 + second), // Constantly increasing
			LabelsMap: obs.Labels{
				"stream_id": "test_stream",
			},
			TimestampUnix: time.Now().Add(time.Duration(second) * time.Second),
		}
		if err := exporter.Export([]obs.DataPoint{metric}); err != nil {
			t.Fatalf("Export failed: %v", err)
		}
	}

	exporter.ForceFlush()

	// Should have 1 metric (updated), but has 3600 (accumulated)
	if len(exporter.collector.metrics) != 1 {
		t.Errorf("Memory growth: expected 1 metric, got %d", len(exporter.collector.metrics))
	}
}

// TestMetricTTL_NotImplemented shows there's no TTL mechanism.
func TestMetricTTL_NotImplemented(t *testing.T) {
	collector := NewDynamicCollector()

	// Add metric
	collector.UpdateMetric("old_metric", 1.0, map[string]string{"test": "true"}, 0)

	// No cleanup method exists
	t.Log("NOTE: No CleanupStaleMetrics() method exists")
	t.Log("NOTE: No TTL configuration exists")
	t.Log("NOTE: Metrics are never removed from memory")
}

// TestSSEExporter_BrokenAfterFFmpegFix shows SSE stream events are broken.
func TestSSEExporter_BrokenAfterFFmpegFix(t *testing.T) {
	mockBus := &MockEventBus{}
	exporter := &SSEExporter{
		eventBus:      mockBus,
		config:        obs.ExporterConfig{Name: "sse", Enabled: true},
		logLevel:      "info",
		streamMetrics: make(map[string]*StreamMetricsAccumulator),
	}

	// Send individual FFmpeg metrics (how it works now)
	ffmpegMetrics := []obs.DataPoint{
		&obs.MetricPoint{Name: "ffmpeg_fps", Value: 30.0, LabelsMap: obs.Labels{"stream_id": "test"}},
		&obs.MetricPoint{Name: "ffmpeg_dropped_frames_total", Value: 5, LabelsMap: obs.Labels{"stream_id": "test"}},
		&obs.MetricPoint{Name: "ffmpeg_duplicate_frames_total", Value: 100, LabelsMap: obs.Labels{"stream_id": "test"}},
		&obs.MetricPoint{Name: "ffmpeg_processing_speed", Value: 0.95, LabelsMap: obs.Labels{"stream_id": "test"}},
	}

	for _, metric := range ffmpegMetrics {
		if err := exporter.Export([]obs.DataPoint{metric}); err != nil {
			t.Fatalf("Export failed: %v", err)
		}
	}

	// Check what events were broadcast
	captured := mockBus.GetEvents()
	for i, event := range captured {
		streamEvent, ok := event.(events.StreamMetricsEvent)
		if !ok {
			continue
		}
		t.Logf("Event %d: type=%s", i, streamEvent.EventType)
	}

	// The issue is likely that individual FFmpeg metrics aren't being combined
	// into a single stream-metrics event anymore
	hasStreamMetrics := false
	for _, event := range captured {
		streamEvent, ok := event.(events.StreamMetricsEvent)
		if ok && streamEvent.EventType == "stream_metrics" {
			hasStreamMetrics = true
			break
		}
	}

	if !hasStreamMetrics {
		t.Error("BUG: No stream-metrics events generated after FFmpeg fix")
		t.Log("The SSE exporter expects combined stream metrics but now gets individual metrics")
	}
}

type MockSSEBroadcasterSimple struct {
	events []SSEEventSimple
}

type SSEEventSimple struct {
	EventType string
	Data      any
}

func (m *MockSSEBroadcasterSimple) BroadcastEvent(eventType string, data any) error {
	m.events = append(m.events, SSEEventSimple{
		EventType: eventType,
		Data:      data,
	})
	return nil
}
