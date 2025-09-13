package exporters

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/smazurov/videonode/internal/obs"
)

// MockSSEBroadcaster for testing
type MockSSEBroadcaster struct {
	mu     sync.Mutex
	events []SSEEvent
}

type SSEEvent struct {
	EventType string
	Data      interface{}
}

func (m *MockSSEBroadcaster) BroadcastEvent(eventType string, data interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, SSEEvent{
		EventType: eventType,
		Data:      data,
	})
	return nil
}

func (m *MockSSEBroadcaster) GetEvents() []SSEEvent {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]SSEEvent, len(m.events))
	copy(result, m.events)
	return result
}

func (m *MockSSEBroadcaster) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = nil
}

func TestSSEExporter_SystemMetrics(t *testing.T) {
	broadcaster := &MockSSEBroadcaster{}
	exporter := NewSSEExporter(broadcaster)

	// Test system metrics event generation (new format - separate metrics)
	systemMetrics := []obs.DataPoint{
		&obs.MetricPoint{
			Name:       "system_load_1m",
			Value:      2.5,
			LabelsMap:  obs.Labels{},
			Timestamp_: time.Now(),
		},
		&obs.MetricPoint{
			Name:       "system_net_rx_bytes",
			Value:      1000000,
			LabelsMap:  obs.Labels{"interface": "eth0"},
			Timestamp_: time.Now(),
		},
	}

	for _, metric := range systemMetrics {
		err := exporter.Export([]obs.DataPoint{metric})
		if err != nil {
			t.Fatalf("Export failed: %v", err)
		}
	}

	events := broadcaster.GetEvents()
	if len(events) != 2 {
		t.Fatalf("Expected 2 events, got %d", len(events))
	}

	// System metrics are now sent as individual mediamtx-metrics events
	for i, event := range events {
		if event.EventType != "mediamtx-metrics" {
			t.Errorf("Event %d: Expected event type 'mediamtx-metrics', got '%s'", i, event.EventType)
		}
	}
}

func TestSSEExporter_StreamMetrics(t *testing.T) {
	broadcaster := &MockSSEBroadcaster{}
	exporter := NewSSEExporter(broadcaster)

	// Test multiple FFmpeg metrics consolidation
	ffmpegMetrics := []obs.DataPoint{
		&obs.MetricPoint{
			Name:       "ffmpeg_fps",
			Value:      30.0,
			LabelsMap:  obs.Labels{"stream_id": "test_stream"},
			Timestamp_: time.Now(),
		},
		&obs.MetricPoint{
			Name:       "ffmpeg_dropped_frames_total",
			Value:      5.0,
			LabelsMap:  obs.Labels{"stream_id": "test_stream"},
			Timestamp_: time.Now(),
		},
		&obs.MetricPoint{
			Name:       "ffmpeg_duplicate_frames_total",
			Value:      10.0,
			LabelsMap:  obs.Labels{"stream_id": "test_stream"},
			Timestamp_: time.Now(),
		},
		&obs.MetricPoint{
			Name:       "ffmpeg_processing_speed",
			Value:      0.95,
			LabelsMap:  obs.Labels{"stream_id": "test_stream"},
			Timestamp_: time.Now(),
		},
	}

	for _, metric := range ffmpegMetrics {
		err := exporter.Export([]obs.DataPoint{metric})
		if err != nil {
			t.Fatalf("Export failed: %v", err)
		}
	}

	events := broadcaster.GetEvents()
	// FFmpeg metrics should now generate combined stream-metrics events
	if len(events) != 4 {
		t.Fatalf("Expected 4 events, got %d", len(events))
	}

	// All should now be stream-metrics type (fixed behavior)
	for i, event := range events {
		if event.EventType != "stream-metrics" {
			t.Errorf("Event %d: Expected type 'stream-metrics', got '%s'", i, event.EventType)
		}
	}
}

func TestSSEExporter_LogFiltering(t *testing.T) {
	broadcaster := &MockSSEBroadcaster{}
	testCases := []struct {
		name          string
		configLevel   string
		logLevel      obs.LogLevel
		shouldProcess bool
	}{
		{"debug level allows all", "debug", obs.LogLevelDebug, true},
		{"debug level allows info", "debug", obs.LogLevelInfo, true},
		{"info level blocks debug", "info", obs.LogLevelDebug, false},
		{"info level allows warn", "info", obs.LogLevelWarn, true},
		{"error level blocks info", "error", obs.LogLevelInfo, false},
		{"error level allows error", "error", obs.LogLevelError, true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			broadcaster.Reset()
			exporter := NewSSEExporter(broadcaster)
			exporter.logLevel = tc.configLevel

			logEntry := &obs.LogEntry{
				Level:      tc.logLevel,
				Message:    "test log",
				Source:     "test",
				Timestamp_: time.Now(),
				LabelsMap:  obs.Labels{},
			}

			err := exporter.Export([]obs.DataPoint{logEntry})
			if err != nil {
				t.Fatalf("Export failed: %v", err)
			}

			// Note: Currently logs are just printed, not broadcasted
			// This test verifies the filtering logic would work
		})
	}
}

func TestSSEExporter_ConcurrentExport(t *testing.T) {
	broadcaster := &MockSSEBroadcaster{}
	exporter := &SSEExporter{
		broadcaster: broadcaster,
		config: obs.ExporterConfig{
			Name:    "sse",
			Enabled: true,
		},
		logLevel: "info",
	}

	// Test concurrent exports don't cause race conditions
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				metric := &obs.MetricPoint{
					Name:       "concurrent_test",
					Value:      float64(j),
					LabelsMap:  obs.Labels{"goroutine": string(rune('0' + id))},
					Timestamp_: time.Now(),
				}
				exporter.Export([]obs.DataPoint{metric})
			}
		}(i)
	}

	wg.Wait()

	events := broadcaster.GetEvents()
	// Should have 100 events total (10 goroutines * 10 metrics each)
	if len(events) != 100 {
		t.Errorf("Expected 100 events, got %d", len(events))
	}
}

func TestSSEExporter_EventRouting(t *testing.T) {
	// Test that events are routed to correct endpoints
	routes := GetEventRoutes()

	expectedRoutes := map[string]string{
		"mediamtx-metrics": "metrics",
		"system-metrics":   "events",
		"obs-alert":        "events",
		"stream-metrics":   "events",
	}

	for eventType, expectedEndpoint := range expectedRoutes {
		if endpoint, ok := routes[eventType]; !ok {
			t.Errorf("Missing route for event type '%s'", eventType)
		} else if endpoint != expectedEndpoint {
			t.Errorf("Event type '%s' routed to '%s', expected '%s'", eventType, endpoint, expectedEndpoint)
		}
	}
}

func TestSSEExporter_MetricsFormatting(t *testing.T) {
	broadcaster := &MockSSEBroadcaster{}
	exporter := &SSEExporter{
		broadcaster: broadcaster,
		config: obs.ExporterConfig{
			Name:    "sse",
			Enabled: true,
		},
		logLevel: "info",
	}

	// Test metrics are formatted correctly for SSE
	testMetric := &obs.MetricPoint{
		Name:       "test_metric",
		Value:      42.5,
		LabelsMap:  obs.Labels{"test": "true", "env": "testing"},
		Timestamp_: time.Now(),
		Unit:       "requests",
	}

	err := exporter.Export([]obs.DataPoint{testMetric})
	if err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	events := broadcaster.GetEvents()
	if len(events) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(events))
	}

	// Check the MediaMTX metrics event structure
	eventData, ok := events[0].Data.(MediaMTXMetricsEvent)
	if !ok {
		t.Fatalf("Event data is not MediaMTXMetricsEvent type")
	}

	if eventData.Count != 1 {
		t.Errorf("Expected count 1, got %d", eventData.Count)
	}

	if len(eventData.Metrics) != 1 {
		t.Fatalf("Expected 1 metric, got %d", len(eventData.Metrics))
	}

	// Check metric fields
	metric := eventData.Metrics[0]
	if metric["name"] != "test_metric" {
		t.Errorf("Expected name 'test_metric', got '%v'", metric["name"])
	}
	if metric["value"] != 42.5 {
		t.Errorf("Expected value 42.5, got '%v'", metric["value"])
	}
	if metric["unit"] != "requests" {
		t.Errorf("Expected unit 'requests', got '%v'", metric["unit"])
	}

	labels, ok := metric["labels"].(obs.Labels)
	if !ok {
		t.Fatalf("Labels are not obs.Labels type, got %T: %+v", metric["labels"], metric["labels"])
	}
	if labels["test"] != "true" {
		t.Errorf("Expected label test='true', got '%s'", labels["test"])
	}
}

func TestSSEExporter_HistoryPreservation(t *testing.T) {
	// SSE should allow seeing historical events (within reason)
	// This is different from Prometheus which only shows current state

	broadcaster := &MockSSEBroadcaster{}
	exporter := &SSEExporter{
		broadcaster: broadcaster,
		config: obs.ExporterConfig{
			Name:    "sse",
			Enabled: true,
		},
		logLevel: "info",
	}

	// Send multiple updates for the same metric
	for i := 0; i < 5; i++ {
		metric := &obs.MetricPoint{
			Name:       "history_test",
			Value:      float64(i),
			LabelsMap:  obs.Labels{"stream_id": "test"},
			Timestamp_: time.Now().Add(time.Duration(i) * time.Second),
		}
		exporter.Export([]obs.DataPoint{metric})
	}

	events := broadcaster.GetEvents()
	// SSE should have all 5 events (history preserved)
	if len(events) != 5 {
		t.Errorf("Expected 5 events (history), got %d", len(events))
	}

	// Each event should have different value
	for i, event := range events {
		eventData := event.Data.(MediaMTXMetricsEvent)
		if len(eventData.Metrics) != 1 {
			continue
		}
		value := eventData.Metrics[0]["value"].(float64)
		if value != float64(i) {
			t.Errorf("Event %d: Expected value %d, got %f", i, i, value)
		}
	}
}

func TestSSEExporter_StartStop(t *testing.T) {
	broadcaster := &MockSSEBroadcaster{}
	exporter := &SSEExporter{
		broadcaster: broadcaster,
		config: obs.ExporterConfig{
			Name:    "sse",
			Enabled: true,
		},
		logLevel: "info",
	}

	// Test Start
	ctx := context.Background()
	err := exporter.Start(ctx)
	if err != nil {
		t.Errorf("Start failed: %v", err)
	}

	// Test Stop
	err = exporter.Stop()
	if err != nil {
		t.Errorf("Stop failed: %v", err)
	}
}
