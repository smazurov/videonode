package events

import (
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/smazurov/videonode/internal/api/models"
)

func TestBus_PublishSubscribe(t *testing.T) {
	bus := New()
	received := make(chan CaptureSuccessEvent, 1)

	unsub := bus.Subscribe(func(e CaptureSuccessEvent) {
		received <- e
	})
	defer unsub()

	event := CaptureSuccessEvent{
		DevicePath: "/dev/video0",
		Message:    "test",
		ImageData:  "data",
		Timestamp:  "2025-01-27T10:30:00Z",
	}
	bus.Publish(event)

	got := <-received
	if got.DevicePath != event.DevicePath {
		t.Errorf("Expected device_path %s, got %s", event.DevicePath, got.DevicePath)
	}
}

func TestBus_MultipleSubscribers(_ *testing.T) {
	bus := New()
	received1 := make(chan StreamCreatedEvent, 1)
	received2 := make(chan StreamCreatedEvent, 1)

	unsub1 := bus.Subscribe(func(e StreamCreatedEvent) {
		received1 <- e
	})
	defer unsub1()

	unsub2 := bus.Subscribe(func(e StreamCreatedEvent) {
		received2 <- e
	})
	defer unsub2()

	event := StreamCreatedEvent{
		Stream: models.StreamData{StreamID: "test"},
		Action: "created",
	}
	bus.Publish(event)

	<-received1
	<-received2
}

func TestBus_Unsubscribe(t *testing.T) {
	bus := New()
	received := make(chan CaptureErrorEvent, 1)

	unsub := bus.Subscribe(func(e CaptureErrorEvent) {
		received <- e
	})

	bus.Publish(CaptureErrorEvent{DevicePath: "/dev/video0"})
	<-received

	unsub()

	bus.Publish(CaptureErrorEvent{DevicePath: "/dev/video1"})
	select {
	case <-received:
		t.Fatal("Should not have received event after unsubscribe")
	case <-time.After(10 * time.Millisecond):
		// Expected - no event
	}
}

func TestBus_TypeSafety(t *testing.T) {
	bus := New()

	captureReceived := make(chan bool, 1)
	streamReceived := make(chan bool, 1)

	unsub1 := bus.Subscribe(func(_ CaptureSuccessEvent) {
		captureReceived <- true
	})
	defer unsub1()

	unsub2 := bus.Subscribe(func(_ StreamCreatedEvent) {
		streamReceived <- true
	})
	defer unsub2()

	// Publish CaptureSuccessEvent
	bus.Publish(CaptureSuccessEvent{DevicePath: "/dev/video0"})
	<-captureReceived

	select {
	case <-streamReceived:
		t.Fatal("Stream subscriber should NOT have received CaptureSuccessEvent")
	case <-time.After(10 * time.Millisecond):
		// Expected
	}

	// Publish StreamCreatedEvent
	bus.Publish(StreamCreatedEvent{Action: "created"})
	<-streamReceived

	select {
	case <-captureReceived:
		t.Fatal("Capture subscriber should NOT have received StreamCreatedEvent")
	case <-time.After(10 * time.Millisecond):
		// Expected
	}
}

func TestBus_ThreadSafety(_ *testing.T) {
	bus := New()
	var wg sync.WaitGroup
	numGoroutines := 10
	eventsPerGoroutine := 100
	expected := numGoroutines * eventsPerGoroutine

	receivedCh := make(chan bool, expected)

	unsub := bus.Subscribe(func(_ DeviceDiscoveryEvent) {
		receivedCh <- true
	})
	defer unsub()

	for range numGoroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range eventsPerGoroutine {
				bus.Publish(DeviceDiscoveryEvent{
					Action:    "added",
					Timestamp: time.Now().Format(time.RFC3339),
				})
			}
		}()
	}

	wg.Wait()

	// Read all expected events
	for range expected {
		<-receivedCh
	}
}

func TestBus_AllEventTypes(t *testing.T) {
	bus := New()

	tests := []struct {
		name  string
		event Event
	}{
		{"CaptureSuccess", CaptureSuccessEvent{DevicePath: "/dev/video0"}},
		{"CaptureError", CaptureErrorEvent{DevicePath: "/dev/video0"}},
		{"DeviceDiscovery", DeviceDiscoveryEvent{Action: "added"}},
		{"StreamCreated", StreamCreatedEvent{Action: "created"}},
		{"StreamUpdated", StreamUpdatedEvent{Action: "updated"}},
		{"StreamDeleted", StreamDeletedEvent{StreamID: "test"}},
		{"StreamStateChanged", StreamStateChangedEvent{StreamID: "test", Enabled: true}},
		{"MediaMTXMetrics", MediaMTXMetricsEvent{EventType: "mediamtx_metrics"}},
		{"OBSAlert", OBSAlertEvent{EventType: "alert"}},
		{"StreamMetrics", StreamMetricsEvent{EventType: "stream_metrics"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(_ *testing.T) {
			received := make(chan Event, 1)

			var unsub func()
			switch tt.event.(type) {
			case CaptureSuccessEvent:
				unsub = bus.Subscribe(func(e CaptureSuccessEvent) { received <- e })
			case CaptureErrorEvent:
				unsub = bus.Subscribe(func(e CaptureErrorEvent) { received <- e })
			case DeviceDiscoveryEvent:
				unsub = bus.Subscribe(func(e DeviceDiscoveryEvent) { received <- e })
			case StreamCreatedEvent:
				unsub = bus.Subscribe(func(e StreamCreatedEvent) { received <- e })
			case StreamUpdatedEvent:
				unsub = bus.Subscribe(func(e StreamUpdatedEvent) { received <- e })
			case StreamDeletedEvent:
				unsub = bus.Subscribe(func(e StreamDeletedEvent) { received <- e })
			case StreamStateChangedEvent:
				unsub = bus.Subscribe(func(e StreamStateChangedEvent) { received <- e })
			case MediaMTXMetricsEvent:
				unsub = bus.Subscribe(func(e MediaMTXMetricsEvent) { received <- e })
			case OBSAlertEvent:
				unsub = bus.Subscribe(func(e OBSAlertEvent) { received <- e })
			case StreamMetricsEvent:
				unsub = bus.Subscribe(func(e StreamMetricsEvent) { received <- e })
			}
			defer unsub()

			bus.Publish(tt.event)
			<-received
		})
	}
}

func TestEventJSONSerialization(t *testing.T) {
	tests := []struct {
		name  string
		event any
	}{
		{
			"CaptureSuccessEvent",
			CaptureSuccessEvent{
				DevicePath: "/dev/video0",
				Message:    "Success",
				ImageData:  "base64data",
				Timestamp:  "2025-01-27T10:30:00Z",
			},
		},
		{
			"StreamCreatedEvent",
			StreamCreatedEvent{
				Stream:    models.StreamData{StreamID: "test-stream"},
				Action:    "created",
				Timestamp: "2025-01-27T10:30:00Z",
			},
		},
		{
			"StreamStateChangedEvent",
			StreamStateChangedEvent{
				StreamID:  "test-stream",
				Enabled:   true,
				Timestamp: "2025-01-27T10:30:00Z",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.event)
			if err != nil {
				t.Fatalf("Failed to marshal: %v", err)
			}

			var result map[string]any
			if unmarshalErr := json.Unmarshal(data, &result); unmarshalErr != nil {
				t.Fatalf("Failed to unmarshal: %v", unmarshalErr)
			}

			if len(result) == 0 {
				t.Fatal("Unmarshaled to empty object")
			}
		})
	}
}

func TestStreamStateChangedEvent_Interface(t *testing.T) {
	event := StreamStateChangedEvent{
		StreamID:  "test-123",
		Enabled:   true,
		Timestamp: "2025-01-27T10:30:00Z",
	}

	if event.GetStreamID() != "test-123" {
		t.Errorf("Expected stream_id test-123, got %s", event.GetStreamID())
	}

	if !event.IsEnabled() {
		t.Error("Expected enabled to be true")
	}
}

func TestSubscribeToChannel(t *testing.T) {
	bus := New()
	ch := make(chan any, 10)

	unsub := SubscribeToChannel[CaptureSuccessEvent](bus, ch)
	defer unsub()

	event := CaptureSuccessEvent{
		DevicePath: "/dev/video0",
		Message:    "test",
	}
	bus.Publish(event)

	received := <-ch
	captureEvent, ok := received.(CaptureSuccessEvent)
	if !ok {
		t.Fatalf("Expected CaptureSuccessEvent, got %T", received)
	}
	if captureEvent.DevicePath != event.DevicePath {
		t.Errorf("Expected device_path %s, got %s", event.DevicePath, captureEvent.DevicePath)
	}
}

func TestSubscribeToChannel_NonBlocking(_ *testing.T) {
	bus := New()
	ch := make(chan any) // No buffer

	unsub := SubscribeToChannel[StreamCreatedEvent](bus, ch)
	defer unsub()

	done := make(chan bool, 1)
	go func() {
		bus.Publish(StreamCreatedEvent{Action: "created"})
		done <- true
	}()

	<-done // Should complete without blocking
}
