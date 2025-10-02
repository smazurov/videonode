package exporters

import (
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/smazurov/videonode/internal/events"
	"github.com/smazurov/videonode/internal/obs"
)

func TestSSEExporter_SystemMetrics(t *testing.T) {
	mockBus := &MockEventBus{}
	exporter := &SSEExporter{
		eventBus:      mockBus,
		config:        obs.ExporterConfig{Name: "sse", Enabled: true},
		logLevel:      "info",
		streamMetrics: make(map[string]*StreamMetricsAccumulator),
	}

	testMetrics := []obs.DataPoint{
		&obs.MetricPoint{
			Name:       "test_value_1",
			Value:      2.5,
			LabelsMap:  obs.Labels{},
			Timestamp_: time.Now(),
		},
		&obs.MetricPoint{
			Name:       "test_bytes",
			Value:      1000000,
			LabelsMap:  obs.Labels{"interface": "eth0"},
			Timestamp_: time.Now(),
		},
	}

	for _, metric := range testMetrics {
		err := exporter.Export([]obs.DataPoint{metric})
		if err != nil {
			t.Fatalf("Export failed: %v", err)
		}
	}

	captured := mockBus.GetEvents()
	if len(captured) != 2 {
		t.Fatalf("Expected 2 events, got %d", len(captured))
	}

	for i, event := range captured {
		if _, ok := event.(events.MediaMTXMetricsEvent); !ok {
			t.Errorf("Event %d: Expected MediaMTXMetricsEvent, got %T", i, event)
		}
	}
}

func TestSSEExporter_StreamMetrics(t *testing.T) {
	mockBus := &MockEventBus{}
	exporter := &SSEExporter{
		eventBus:      mockBus,
		config:        obs.ExporterConfig{Name: "sse", Enabled: true},
		logLevel:      "info",
		streamMetrics: make(map[string]*StreamMetricsAccumulator),
	}

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

	captured := mockBus.GetEvents()
	if len(captured) != 4 {
		t.Fatalf("Expected 4 events, got %d", len(captured))
	}

	for i, event := range captured {
		if _, ok := event.(events.StreamMetricsEvent); !ok {
			t.Errorf("Event %d: Expected StreamMetricsEvent, got %T", i, event)
		}
	}
}

func TestSSEExporter_LogFiltering(t *testing.T) {
	mockBus := &MockEventBus{}
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
			mockBus.Reset()
			exporter := &SSEExporter{
				eventBus:      mockBus,
				config:        obs.ExporterConfig{Name: "sse", Enabled: true},
				logger:        slog.Default(),
				logLevel:      tc.configLevel,
				streamMetrics: make(map[string]*StreamMetricsAccumulator),
			}

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
		})
	}
}

func TestSSEExporter_ConcurrentExport(t *testing.T) {
	mockBus := &MockEventBus{}
	exporter := &SSEExporter{
		eventBus:      mockBus,
		config:        obs.ExporterConfig{Name: "sse", Enabled: true},
		logLevel:      "info",
		streamMetrics: make(map[string]*StreamMetricsAccumulator),
	}

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
	captured := mockBus.GetEvents()

	if len(captured) != 100 {
		t.Errorf("Expected 100 events, got %d", len(captured))
	}
}

func TestSSEExporter_EventRouting(t *testing.T) {
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
	mockBus := &MockEventBus{}
	exporter := &SSEExporter{
		eventBus:      mockBus,
		config:        obs.ExporterConfig{Name: "sse", Enabled: true},
		logLevel:      "info",
		streamMetrics: make(map[string]*StreamMetricsAccumulator),
	}

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

	captured := mockBus.GetEvents()
	if len(captured) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(captured))
	}

	eventData, ok := captured[0].(events.MediaMTXMetricsEvent)
	if !ok {
		t.Fatalf("Event data is not MediaMTXMetricsEvent type, got %T", captured[0])
	}

	if eventData.Count != 1 {
		t.Errorf("Expected count 1, got %d", eventData.Count)
	}

	if len(eventData.Metrics) != 1 {
		t.Fatalf("Expected 1 metric, got %d", len(eventData.Metrics))
	}

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
	mockBus := &MockEventBus{}
	exporter := &SSEExporter{
		eventBus:      mockBus,
		config:        obs.ExporterConfig{Name: "sse", Enabled: true},
		logLevel:      "info",
		streamMetrics: make(map[string]*StreamMetricsAccumulator),
	}

	for i := 0; i < 5; i++ {
		metric := &obs.MetricPoint{
			Name:       "history_test",
			Value:      float64(i),
			LabelsMap:  obs.Labels{"stream_id": "test"},
			Timestamp_: time.Now().Add(time.Duration(i) * time.Second),
		}
		exporter.Export([]obs.DataPoint{metric})
	}

	captured := mockBus.GetEvents()
	if len(captured) != 5 {
		t.Errorf("Expected 5 events (history), got %d", len(captured))
	}

	for i, event := range captured {
		eventData := event.(events.MediaMTXMetricsEvent)
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
	mockBus := &MockEventBus{}
	exporter := &SSEExporter{
		eventBus:      mockBus,
		config:        obs.ExporterConfig{Name: "sse", Enabled: true},
		logLevel:      "info",
		streamMetrics: make(map[string]*StreamMetricsAccumulator),
	}

	ctx := context.Background()
	err := exporter.Start(ctx)
	if err != nil {
		t.Errorf("Start failed: %v", err)
	}

	err = exporter.Stop()
	if err != nil {
		t.Errorf("Stop failed: %v", err)
	}
}
