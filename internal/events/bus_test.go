package events

import (
	"encoding/json"
	"sync"
	"testing"
	"time"
)

func TestBus_PublishSubscribe(t *testing.T) {
	bus := New()
	received := make(chan DeviceDiscoveryEvent, 1)

	unsub := Subscribe(bus, func(e DeviceDiscoveryEvent) {
		received <- e
	})
	defer unsub()

	ev := DeviceDiscoveryEvent{Action: "added", Timestamp: "2025-01-27T10:30:00Z"}
	Publish(bus, ev)

	got := <-received
	if got.Action != ev.Action {
		t.Errorf("Expected action %s, got %s", ev.Action, got.Action)
	}
}

func TestBus_MultipleSubscribers(_ *testing.T) {
	bus := New()
	received1 := make(chan DeviceDiscoveryEvent, 1)
	received2 := make(chan DeviceDiscoveryEvent, 1)

	unsub1 := Subscribe(bus, func(e DeviceDiscoveryEvent) { received1 <- e })
	defer unsub1()
	unsub2 := Subscribe(bus, func(e DeviceDiscoveryEvent) { received2 <- e })
	defer unsub2()

	Publish(bus, DeviceDiscoveryEvent{Action: "added"})

	<-received1
	<-received2
}

func TestBus_Unsubscribe(t *testing.T) {
	bus := New()
	received := make(chan PipelineStateChangedEvent, 1)

	unsub := Subscribe(bus, func(e PipelineStateChangedEvent) { received <- e })

	Publish(bus, PipelineStateChangedEvent{Enabled: true})
	<-received

	unsub()

	Publish(bus, PipelineStateChangedEvent{Enabled: false})
	select {
	case <-received:
		t.Fatal("Should not have received event after unsubscribe")
	case <-time.After(10 * time.Millisecond):
		// Expected - no event
	}
}

func TestBus_TypeSafety(t *testing.T) {
	bus := New()

	discoveryReceived := make(chan bool, 1)
	pipelineReceived := make(chan bool, 1)

	unsub1 := Subscribe(bus, func(_ DeviceDiscoveryEvent) { discoveryReceived <- true })
	defer unsub1()
	unsub2 := Subscribe(bus, func(_ PipelineStateChangedEvent) { pipelineReceived <- true })
	defer unsub2()

	// Publish DeviceDiscoveryEvent — only the discovery subscriber fires.
	Publish(bus, DeviceDiscoveryEvent{Action: "added"})
	<-discoveryReceived
	select {
	case <-pipelineReceived:
		t.Fatal("Pipeline subscriber should NOT have received DeviceDiscoveryEvent")
	case <-time.After(10 * time.Millisecond):
	}

	// Publish PipelineStateChangedEvent — only the pipeline subscriber fires.
	Publish(bus, PipelineStateChangedEvent{Enabled: true})
	<-pipelineReceived
	select {
	case <-discoveryReceived:
		t.Fatal("Discovery subscriber should NOT have received PipelineStateChangedEvent")
	case <-time.After(10 * time.Millisecond):
	}
}

func TestBus_ThreadSafety(_ *testing.T) {
	bus := New()
	var wg sync.WaitGroup
	numGoroutines := 10
	eventsPerGoroutine := 100
	expected := numGoroutines * eventsPerGoroutine

	receivedCh := make(chan bool, expected)

	unsub := Subscribe(bus, func(_ DeviceDiscoveryEvent) {
		receivedCh <- true
	})
	defer unsub()

	for range numGoroutines {
		wg.Go(func() {
			for range eventsPerGoroutine {
				Publish(bus, DeviceDiscoveryEvent{
					Action:    "added",
					Timestamp: time.Now().Format(time.RFC3339),
				})
			}
		})
	}

	wg.Wait()

	for range expected {
		<-receivedCh
	}
}

// TestBus_RoundTripSurvivingTypes exercises Publish/Subscribe for each event
// type that still flows on the bus after the SSE clean sweep.
func TestBus_RoundTripSurvivingTypes(t *testing.T) {
	bus := New()

	t.Run("DeviceDiscovery", func(_ *testing.T) {
		ch := make(chan DeviceDiscoveryEvent, 1)
		unsub := Subscribe(bus, func(e DeviceDiscoveryEvent) { ch <- e })
		defer unsub()
		Publish(bus, DeviceDiscoveryEvent{Action: "added"})
		<-ch
	})
	t.Run("PipelineStateChanged", func(_ *testing.T) {
		ch := make(chan PipelineStateChangedEvent, 1)
		unsub := Subscribe(bus, func(e PipelineStateChangedEvent) { ch <- e })
		defer unsub()
		Publish(bus, PipelineStateChangedEvent{Enabled: true})
		<-ch
	})
	t.Run("Entity", func(_ *testing.T) {
		ch := make(chan EntityEvent, 1)
		unsub := Subscribe(bus, func(e EntityEvent) { ch <- e })
		defer unsub()
		Publish(bus, EntityEvent{Kind: "source." + ActionCreated, ID: "hdmi0"})
		<-ch
	})
	t.Run("StreamCrashed", func(_ *testing.T) {
		ch := make(chan StreamCrashedEvent, 1)
		unsub := Subscribe(bus, func(e StreamCrashedEvent) { ch <- e })
		defer unsub()
		Publish(bus, StreamCrashedEvent{StreamID: "s1", DeviceID: "hdmi0"})
		<-ch
	})
	t.Run("LogEntry", func(_ *testing.T) {
		ch := make(chan LogEntryEvent, 1)
		unsub := Subscribe(bus, func(e LogEntryEvent) { ch <- e })
		defer unsub()
		Publish(bus, LogEntryEvent{Level: "info", Message: "hi"})
		<-ch
	})
	t.Run("Heartbeat", func(_ *testing.T) {
		ch := make(chan HeartbeatEvent, 1)
		unsub := Subscribe(bus, func(e HeartbeatEvent) { ch <- e })
		defer unsub()
		Publish(bus, HeartbeatEvent{Timestamp: "2025-01-27T10:30:00Z"})
		<-ch
	})
}

func TestEventJSONSerialization(t *testing.T) {
	tests := []struct {
		name  string
		event any
	}{
		{"DeviceDiscoveryEvent", DeviceDiscoveryEvent{Action: "added", Timestamp: "2025-01-27T10:30:00Z"}},
		{"PipelineStateChangedEvent", PipelineStateChangedEvent{Enabled: true, Timestamp: "2025-01-27T10:30:00Z"}},
		{"EntityEvent", EntityEvent{Kind: "source." + ActionUpdated, ID: "hdmi0", Timestamp: "2025-01-27T10:30:00Z"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.event)
			if err != nil {
				t.Fatalf("Failed to marshal: %v", err)
			}

			var result map[string]any
			if err := json.Unmarshal(data, &result); err != nil {
				t.Fatalf("Failed to unmarshal: %v", err)
			}

			if len(result) == 0 {
				t.Fatal("Unmarshaled to empty object")
			}
		})
	}
}

func TestSubscribeToChannel(t *testing.T) {
	bus := New()
	ch := make(chan any, 10)

	unsub := SubscribeToChannel[DeviceDiscoveryEvent](bus, ch)
	defer unsub()

	ev := DeviceDiscoveryEvent{Action: "added"}
	Publish(bus, ev)

	received := <-ch
	discoveryEvent, ok := received.(DeviceDiscoveryEvent)
	if !ok {
		t.Fatalf("Expected DeviceDiscoveryEvent, got %T", received)
	}
	if discoveryEvent.Action != ev.Action {
		t.Errorf("Expected action %s, got %s", ev.Action, discoveryEvent.Action)
	}
}

// TestBus_ReentrantPublish pins the invariant that no bus lock is held while
// handlers run: the logging callback publishes LogEntryEvent, so a handler
// that logs re-enters Publish.
func TestBus_ReentrantPublish(t *testing.T) {
	bus := New()
	logged := make(chan LogEntryEvent, 1)

	unsubLog := Subscribe(bus, func(e LogEntryEvent) { logged <- e })
	defer unsubLog()

	unsub := Subscribe(bus, func(_ DeviceDiscoveryEvent) {
		Publish(bus, LogEntryEvent{Level: "info", Message: "from handler"})
	})
	defer unsub()

	Publish(bus, DeviceDiscoveryEvent{Action: "added"})

	select {
	case <-logged:
	case <-time.After(time.Second):
		t.Fatal("re-entrant Publish did not deliver (deadlock?)")
	}
}

type collidingEvent struct{}

func (collidingEvent) Type() uint32 { return TypeDeviceDiscovery }

func TestBus_DuplicateTypeCodePanics(t *testing.T) {
	bus := New()
	unsub := Subscribe(bus, func(_ DeviceDiscoveryEvent) {})
	defer unsub()

	defer func() {
		if recover() == nil {
			t.Fatal("Subscribe with a colliding Type() code should panic")
		}
	}()
	Subscribe(bus, func(_ collidingEvent) {})
}

func TestSubscribeToChannel_NonBlocking(_ *testing.T) {
	bus := New()
	ch := make(chan any) // No buffer

	unsub := SubscribeToChannel[PipelineStateChangedEvent](bus, ch)
	defer unsub()

	done := make(chan bool, 1)
	go func() {
		Publish(bus, PipelineStateChangedEvent{Enabled: true})
		done <- true
	}()

	<-done // Should complete without blocking
}
