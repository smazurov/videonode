package exporters

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/smazurov/videonode/internal/events"
	"github.com/smazurov/videonode/internal/metrics"
)

type mockEventBus struct {
	mu        sync.Mutex
	events    []events.Event
	published chan struct{}
}

func newMockEventBus() *mockEventBus {
	return &mockEventBus{
		events:    make([]events.Event, 0),
		published: make(chan struct{}, 100),
	}
}

func (m *mockEventBus) Publish(ev events.Event) {
	m.mu.Lock()
	m.events = append(m.events, ev)
	m.mu.Unlock()
	select {
	case m.published <- struct{}{}:
	default:
	}
}

func (m *mockEventBus) getEvents() []events.Event {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]events.Event, len(m.events))
	copy(result, m.events)
	return result
}

func TestSSEExporterPublishesMetrics(t *testing.T) {
	streamID := "sse-test-stream"
	metrics.DeleteFFmpegMetrics(streamID)

	// Set up metrics
	metrics.SetFFmpegFPS(streamID, 30.0)
	metrics.SetFFmpegDroppedFrames(streamID, 5)
	metrics.SetFFmpegDuplicateFrames(streamID, 2)

	mock := newMockEventBus()
	exporter := NewSSEExporter(mock)
	exporter.interval = 50 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	exporter.Start(ctx)

	// Wait for at least one publish cycle
	select {
	case <-mock.published:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("timeout waiting for metrics publish")
	}

	cancel()
	exporter.Stop()

	evts := mock.getEvents()
	if len(evts) == 0 {
		t.Fatal("expected at least one event")
	}

	var found bool
	for _, ev := range evts {
		if sme, ok := ev.(events.StreamMetricsEvent); ok {
			if sme.StreamID == streamID {
				found = true
				if sme.FPS != "30.00" {
					t.Errorf("FPS = %q, want \"30.00\"", sme.FPS)
				}
				if sme.DroppedFrames != "5" {
					t.Errorf("DroppedFrames = %q, want \"5\"", sme.DroppedFrames)
				}
				if sme.DuplicateFrames != "2" {
					t.Errorf("DuplicateFrames = %q, want \"2\"", sme.DuplicateFrames)
				}
				break
			}
		}
	}

	if !found {
		t.Error("expected StreamMetricsEvent for test stream")
	}

	metrics.DeleteFFmpegMetrics(streamID)
}

func TestSSEExporterNoMetrics(t *testing.T) {
	// Use unique stream ID to avoid interference from other tests
	testStreamID := "sse-no-metrics-test"
	metrics.DeleteFFmpegMetrics(testStreamID)

	mock := newMockEventBus()
	exporter := NewSSEExporter(mock)
	exporter.interval = 20 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	exporter.Start(ctx)

	// Wait for at least one publish cycle
	time.Sleep(50 * time.Millisecond)

	cancel()
	exporter.Stop()

	// Verify no events were published for our test stream
	for _, ev := range mock.getEvents() {
		if sme, ok := ev.(events.StreamMetricsEvent); ok {
			if sme.StreamID == testStreamID {
				t.Error("expected no events for deleted stream")
			}
		}
	}
}

func TestSSEExporterStopIdempotent(t *testing.T) {
	streamID := "sse-idempotent-test"
	metrics.SetFFmpegFPS(streamID, 30.0)
	defer metrics.DeleteFFmpegMetrics(streamID)

	mock := newMockEventBus()
	exporter := NewSSEExporter(mock)
	exporter.interval = 10 * time.Millisecond

	ctx := context.Background()
	exporter.Start(ctx)

	// Let it run briefly
	time.Sleep(30 * time.Millisecond)

	// Stop multiple times
	exporter.Stop()
	exporter.Stop()
	exporter.Stop()

	// Record event count after stops
	countAfterStop := len(mock.getEvents())

	// Wait and verify no new events after stop
	time.Sleep(30 * time.Millisecond)
	countAfterWait := len(mock.getEvents())

	if countAfterWait != countAfterStop {
		t.Errorf("events published after stop: got %d, want %d", countAfterWait, countAfterStop)
	}
}

func TestSSEExporterStopBeforeStart(t *testing.T) {
	streamID := "sse-stop-before-start-test"
	metrics.SetFFmpegFPS(streamID, 45.0)
	defer metrics.DeleteFFmpegMetrics(streamID)

	mock := newMockEventBus()
	exporter := NewSSEExporter(mock)
	exporter.interval = 10 * time.Millisecond

	// Stop before start should not panic
	exporter.Stop()

	// Should still be able to start and function normally
	ctx := t.Context()
	exporter.Start(ctx)

	// Wait for publish cycle
	time.Sleep(30 * time.Millisecond)
	exporter.Stop()

	// Verify events were published after start
	if len(mock.getEvents()) == 0 {
		t.Error("expected events after Start(), got none")
	}
}

func TestGetEventTypes(t *testing.T) {
	types := GetEventTypes()
	if _, ok := types["stream-metrics"]; !ok {
		t.Error("expected stream-metrics event type")
	}
}

func TestGetEventTypesForEndpoint(t *testing.T) {
	types := GetEventTypesForEndpoint("events")
	if _, ok := types["stream-metrics"]; !ok {
		t.Error("expected stream-metrics for events endpoint")
	}

	types = GetEventTypesForEndpoint("unknown")
	if len(types) != 0 {
		t.Error("expected empty map for unknown endpoint")
	}
}

func TestGetEventRoutes(t *testing.T) {
	routes := GetEventRoutes()
	if routes["stream-metrics"] != "events" {
		t.Errorf("stream-metrics route = %q, want \"events\"", routes["stream-metrics"])
	}
}
