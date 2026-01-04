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
			Name:          "ffmpeg_fps",
			Value:         30.0,
			LabelsMap:     obs.Labels{"stream_id": "test_stream"},
			TimestampUnix: time.Now(),
		},
		&obs.MetricPoint{
			Name:          "ffmpeg_dropped_frames_total",
			Value:         5.0,
			LabelsMap:     obs.Labels{"stream_id": "test_stream"},
			TimestampUnix: time.Now(),
		},
		&obs.MetricPoint{
			Name:          "ffmpeg_duplicate_frames_total",
			Value:         10.0,
			LabelsMap:     obs.Labels{"stream_id": "test_stream"},
			TimestampUnix: time.Now(),
		},
		&obs.MetricPoint{
			Name:          "ffmpeg_processing_speed",
			Value:         0.95,
			LabelsMap:     obs.Labels{"stream_id": "test_stream"},
			TimestampUnix: time.Now(),
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
				Level:         tc.logLevel,
				Message:       "test log",
				Source:        "test",
				TimestampUnix: time.Now(),
				LabelsMap:     obs.Labels{},
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
	for i := range 10 {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := range 10 {
				metric := &obs.MetricPoint{
					Name:          "ffmpeg_fps",
					Value:         float64(j),
					LabelsMap:     obs.Labels{"stream_id": string(rune('a' + id))},
					TimestampUnix: time.Now(),
				}
				if err := exporter.Export([]obs.DataPoint{metric}); err != nil {
					t.Errorf("Export failed: %v", err)
				}
			}
		}(i)
	}

	wg.Wait()
	captured := mockBus.GetEvents()

	// Each goroutine sends 10 FFmpeg metrics, each produces a StreamMetricsEvent
	if len(captured) != 100 {
		t.Errorf("Expected 100 events, got %d", len(captured))
	}
}

func TestSSEExporter_EventRouting(t *testing.T) {
	routes := GetEventRoutes()

	expectedRoutes := map[string]string{
		"stream-metrics": "events",
	}

	for eventType, expectedEndpoint := range expectedRoutes {
		if endpoint, ok := routes[eventType]; !ok {
			t.Errorf("Missing route for event type '%s'", eventType)
		} else if endpoint != expectedEndpoint {
			t.Errorf("Event type '%s' routed to '%s', expected '%s'", eventType, endpoint, expectedEndpoint)
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
