package exporters

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/smazurov/videonode/internal/events"
	"github.com/smazurov/videonode/internal/metrics"
	"github.com/smazurov/videonode/internal/streaming"
)

type capturedEvent struct {
	entityType string
	action     string
	id         string
	payload    any
}

// mockRegistry implements the EntityPublisher interface, capturing the
// entity envelopes the exporter publishes.
type mockRegistry struct {
	mu        sync.Mutex
	events    []capturedEvent
	published chan struct{}
}

func newMockRegistry() *mockRegistry {
	return &mockRegistry{published: make(chan struct{}, 100)}
}

func (m *mockRegistry) Publish(entityType, action, id string, payload any) {
	m.mu.Lock()
	m.events = append(m.events, capturedEvent{entityType, action, id, payload})
	m.mu.Unlock()
	select {
	case m.published <- struct{}{}:
	default:
	}
}

func (m *mockRegistry) getEvents() []capturedEvent {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]capturedEvent, len(m.events))
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

	mock := newMockRegistry()
	exporter := NewSSEExporter(mock, nil)
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
		if ev.entityType != "stream" || ev.action != events.ActionMetrics || ev.id != streamID {
			continue
		}
		found = true
		payload, ok := ev.payload.(streaming.StreamMetricsPayload)
		if !ok {
			t.Fatalf("payload type = %T, want streaming.StreamMetricsPayload", ev.payload)
		}
		if payload.FPS != 30.0 {
			t.Errorf("fps = %v, want 30", payload.FPS)
		}
		if payload.DroppedFrames != 5 {
			t.Errorf("dropped_frames = %v, want 5", payload.DroppedFrames)
		}
		if payload.DuplicateFrames != 2 {
			t.Errorf("duplicate_frames = %v, want 2", payload.DuplicateFrames)
		}
		break
	}

	if !found {
		t.Error("expected stream metrics entity event for test stream")
	}

	metrics.DeleteFFmpegMetrics(streamID)
}

func TestSSEExporterNoMetrics(t *testing.T) {
	// Use unique stream ID to avoid interference from other tests
	testStreamID := "sse-no-metrics-test"
	metrics.DeleteFFmpegMetrics(testStreamID)

	mock := newMockRegistry()
	exporter := NewSSEExporter(mock, nil)
	exporter.interval = 20 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	exporter.Start(ctx)

	// Wait for at least one publish cycle
	time.Sleep(50 * time.Millisecond)

	cancel()
	exporter.Stop()

	// Verify no events were published for our test stream
	for _, ev := range mock.getEvents() {
		if ev.entityType == "stream" && ev.id == testStreamID {
			t.Error("expected no events for deleted stream")
		}
	}
}

func TestSSEExporterStopIdempotent(t *testing.T) {
	streamID := "sse-idempotent-test"
	metrics.SetFFmpegFPS(streamID, 30.0)
	defer metrics.DeleteFFmpegMetrics(streamID)

	mock := newMockRegistry()
	exporter := NewSSEExporter(mock, nil)
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

	mock := newMockRegistry()
	exporter := NewSSEExporter(mock, nil)
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
