package api

import (
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/smazurov/videonode/internal/api/models"
)

func TestEventBroadcaster_Subscribe(t *testing.T) {
	broadcaster := &EventBroadcaster{
		channels: make([]chan<- interface{}, 0),
	}

	// Create test channel
	ch := make(chan interface{}, 1)

	// Subscribe
	broadcaster.Subscribe(ch)

	// Verify channel was added
	if len(broadcaster.channels) != 1 {
		t.Errorf("Expected 1 subscriber, got %d", len(broadcaster.channels))
	}
}

func TestEventBroadcaster_Unsubscribe(t *testing.T) {
	broadcaster := &EventBroadcaster{
		channels: make([]chan<- interface{}, 0),
	}

	// Create test channel
	ch := make(chan interface{}, 1)

	// Subscribe then unsubscribe
	broadcaster.Subscribe(ch)
	broadcaster.Unsubscribe(ch)

	// Verify channel was removed
	if len(broadcaster.channels) != 0 {
		t.Errorf("Expected 0 subscribers, got %d", len(broadcaster.channels))
	}
}

func TestEventBroadcaster_Broadcast(t *testing.T) {
	broadcaster := &EventBroadcaster{
		channels: make([]chan<- interface{}, 0),
	}

	// Create test channels
	ch1 := make(chan interface{}, 1)
	ch2 := make(chan interface{}, 1)

	// Subscribe both channels
	broadcaster.Subscribe(ch1)
	broadcaster.Subscribe(ch2)

	// Broadcast test event
	testEvent := "test_event"
	broadcaster.Broadcast(testEvent)

	// Verify both channels received the event
	select {
	case event := <-ch1:
		if event != testEvent {
			t.Errorf("Expected %v, got %v", testEvent, event)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Channel 1 did not receive event")
	}

	select {
	case event := <-ch2:
		if event != testEvent {
			t.Errorf("Expected %v, got %v", testEvent, event)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Channel 2 did not receive event")
	}
}

func TestEventBroadcaster_ThreadSafety(t *testing.T) {
	broadcaster := &EventBroadcaster{
		channels: make([]chan<- interface{}, 0),
	}

	var wg sync.WaitGroup
	numGoroutines := 10

	// Concurrently subscribe and unsubscribe channels
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ch := make(chan interface{}, 1)
			broadcaster.Subscribe(ch)
			broadcaster.Unsubscribe(ch)
		}()
	}

	wg.Wait()

	// Should end with no subscribers
	if len(broadcaster.channels) != 0 {
		t.Errorf("Expected 0 subscribers after concurrent operations, got %d", len(broadcaster.channels))
	}
}

func TestBroadcastStreamCreated(t *testing.T) {
	// Create test channel to capture events
	ch := make(chan interface{}, 1)

	// Subscribe to global broadcaster
	globalEventBroadcaster.Subscribe(ch)
	defer globalEventBroadcaster.Unsubscribe(ch)

	// Create test stream data
	startTime, _ := time.Parse(time.RFC3339, "2025-01-27T10:30:00Z")
	streamData := models.StreamData{
		StreamID:  "test-stream-001",
		DeviceID:  "device-001",
		Codec:     "h264",
		Bitrate:   "2M",
		StartTime: startTime,
		WebRTCURL: "http://localhost:8889/test-stream-001",
		RTSPURL:   "rtsp://localhost:8554/test-stream-001",
	}
	timestamp := "2025-01-27T10:30:00Z"

	// Broadcast stream created event
	BroadcastStreamCreated(streamData, timestamp)

	// Verify event was received
	select {
	case event := <-ch:
		createdEvent, ok := event.(StreamCreatedEvent)
		if !ok {
			t.Errorf("Expected StreamCreatedEvent, got %T", event)
			return
		}

		// Verify event structure
		if createdEvent.Stream.StreamID != streamData.StreamID {
			t.Errorf("Expected stream_id %s, got %s", streamData.StreamID, createdEvent.Stream.StreamID)
		}
		if createdEvent.Action != "created" {
			t.Errorf("Expected action 'created', got %s", createdEvent.Action)
		}
		if createdEvent.Timestamp != timestamp {
			t.Errorf("Expected timestamp %s, got %s", timestamp, createdEvent.Timestamp)
		}

	case <-time.After(100 * time.Millisecond):
		t.Error("Did not receive stream created event")
	}
}

func TestBroadcastStreamDeleted(t *testing.T) {
	// Create test channel to capture events
	ch := make(chan interface{}, 1)

	// Subscribe to global broadcaster
	globalEventBroadcaster.Subscribe(ch)
	defer globalEventBroadcaster.Unsubscribe(ch)

	streamID := "test-stream-001"
	timestamp := "2025-01-27T10:30:00Z"

	// Broadcast stream deleted event
	BroadcastStreamDeleted(streamID, timestamp)

	// Verify event was received
	select {
	case event := <-ch:
		deletedEvent, ok := event.(StreamDeletedEvent)
		if !ok {
			t.Errorf("Expected StreamDeletedEvent, got %T", event)
			return
		}

		// Verify event structure
		if deletedEvent.StreamID != streamID {
			t.Errorf("Expected stream_id %s, got %s", streamID, deletedEvent.StreamID)
		}
		if deletedEvent.Action != "deleted" {
			t.Errorf("Expected action 'deleted', got %s", deletedEvent.Action)
		}
		if deletedEvent.Timestamp != timestamp {
			t.Errorf("Expected timestamp %s, got %s", timestamp, deletedEvent.Timestamp)
		}

	case <-time.After(100 * time.Millisecond):
		t.Error("Did not receive stream deleted event")
	}
}

func TestBroadcastStreamUpdated(t *testing.T) {
	// Create test channel to capture events
	ch := make(chan interface{}, 1)

	// Subscribe to global broadcaster
	globalEventBroadcaster.Subscribe(ch)
	defer globalEventBroadcaster.Unsubscribe(ch)

	// Create test stream data
	startTime, _ := time.Parse(time.RFC3339, "2025-01-27T10:30:00Z")
	streamData := models.StreamData{
		StreamID:  "test-stream-001",
		DeviceID:  "device-001",
		Codec:     "h265",
		Bitrate:   "4M",
		StartTime: startTime,
		WebRTCURL: "http://localhost:8889/test-stream-001",
		RTSPURL:   "rtsp://localhost:8554/test-stream-001",
	}
	timestamp := "2025-01-27T10:30:00Z"

	// Broadcast stream updated event
	BroadcastStreamUpdated(streamData, timestamp)

	// Verify event was received
	select {
	case event := <-ch:
		updatedEvent, ok := event.(StreamUpdatedEvent)
		if !ok {
			t.Errorf("Expected StreamUpdatedEvent, got %T", event)
			return
		}

		// Verify event structure
		if updatedEvent.Stream.StreamID != streamData.StreamID {
			t.Errorf("Expected stream_id %s, got %s", streamData.StreamID, updatedEvent.Stream.StreamID)
		}
		if updatedEvent.Action != "updated" {
			t.Errorf("Expected action 'updated', got %s", updatedEvent.Action)
		}
		if updatedEvent.Timestamp != timestamp {
			t.Errorf("Expected timestamp %s, got %s", timestamp, updatedEvent.Timestamp)
		}

	case <-time.After(100 * time.Millisecond):
		t.Error("Did not receive stream updated event")
	}
}

func TestBroadcastDeviceDiscovery(t *testing.T) {
	// Create test channel to capture events
	ch := make(chan interface{}, 1)

	// Subscribe to global broadcaster
	globalEventBroadcaster.Subscribe(ch)
	defer globalEventBroadcaster.Unsubscribe(ch)

	device := models.DeviceInfo{
		DevicePath:   "/dev/video0",
		DeviceName:   "Test Camera",
		DeviceId:     "test-device-001",
		Caps:         1234,
		Capabilities: []string{"video_capture", "streaming"},
	}
	action := "added"
	timestamp := "2025-01-27T10:30:00Z"

	// Broadcast device discovery event
	BroadcastDeviceDiscovery(action, device, timestamp)

	// Verify event was received
	select {
	case event := <-ch:
		discoveryEvent, ok := event.(DeviceDiscoveryEvent)
		if !ok {
			t.Errorf("Expected DeviceDiscoveryEvent, got %T", event)
			return
		}

		// Verify event structure
		if discoveryEvent.DevicePath != device.DevicePath {
			t.Errorf("Expected device_path %s, got %s", device.DevicePath, discoveryEvent.DevicePath)
		}
		if discoveryEvent.Action != action {
			t.Errorf("Expected action %s, got %s", action, discoveryEvent.Action)
		}
		if discoveryEvent.Timestamp != timestamp {
			t.Errorf("Expected timestamp %s, got %s", timestamp, discoveryEvent.Timestamp)
		}

	case <-time.After(100 * time.Millisecond):
		t.Error("Did not receive device discovery event")
	}
}

func TestBroadcastCaptureSuccess(t *testing.T) {
	// Create test channel to capture events
	ch := make(chan interface{}, 1)

	// Subscribe to global broadcaster
	globalEventBroadcaster.Subscribe(ch)
	defer globalEventBroadcaster.Unsubscribe(ch)

	devicePath := "/dev/video0"
	imageData := "base64encodedimage=="
	timestamp := "2025-01-27T10:30:00Z"

	// Broadcast capture success event
	BroadcastCaptureSuccess(devicePath, imageData, timestamp)

	// Verify event was received
	select {
	case event := <-ch:
		captureEvent, ok := event.(CaptureSuccessEvent)
		if !ok {
			t.Errorf("Expected CaptureSuccessEvent, got %T", event)
			return
		}

		// Verify event structure
		if captureEvent.DevicePath != devicePath {
			t.Errorf("Expected device_path %s, got %s", devicePath, captureEvent.DevicePath)
		}
		if captureEvent.ImageData != imageData {
			t.Errorf("Expected image_data %s, got %s", imageData, captureEvent.ImageData)
		}
		if captureEvent.Timestamp != timestamp {
			t.Errorf("Expected timestamp %s, got %s", timestamp, captureEvent.Timestamp)
		}
		if captureEvent.Message != "Screenshot captured successfully" {
			t.Errorf("Expected success message, got %s", captureEvent.Message)
		}

	case <-time.After(100 * time.Millisecond):
		t.Error("Did not receive capture success event")
	}
}

func TestBroadcastCaptureError(t *testing.T) {
	// Create test channel to capture events
	ch := make(chan interface{}, 1)

	// Subscribe to global broadcaster
	globalEventBroadcaster.Subscribe(ch)
	defer globalEventBroadcaster.Unsubscribe(ch)

	devicePath := "/dev/video0"
	errorMsg := "Device not found"
	timestamp := "2025-01-27T10:30:00Z"

	// Broadcast capture error event
	BroadcastCaptureError(devicePath, errorMsg, timestamp)

	// Verify event was received
	select {
	case event := <-ch:
		captureEvent, ok := event.(CaptureErrorEvent)
		if !ok {
			t.Errorf("Expected CaptureErrorEvent, got %T", event)
			return
		}

		// Verify event structure
		if captureEvent.DevicePath != devicePath {
			t.Errorf("Expected device_path %s, got %s", devicePath, captureEvent.DevicePath)
		}
		if captureEvent.Error != errorMsg {
			t.Errorf("Expected error %s, got %s", errorMsg, captureEvent.Error)
		}
		if captureEvent.Timestamp != timestamp {
			t.Errorf("Expected timestamp %s, got %s", timestamp, captureEvent.Timestamp)
		}
		if captureEvent.Message != "Screenshot capture failed" {
			t.Errorf("Expected error message, got %s", captureEvent.Message)
		}

	case <-time.After(100 * time.Millisecond):
		t.Error("Did not receive capture error event")
	}
}

func TestEventJSONSerialization(t *testing.T) {
	// Test that events can be properly serialized to JSON as expected by frontend

	t.Run("StreamCreatedEvent", func(t *testing.T) {
		startTime, _ := time.Parse(time.RFC3339, "2025-01-27T10:30:00Z")
		event := StreamCreatedEvent{
			Stream: models.StreamData{
				StreamID:  "test-stream",
				DeviceID:  "device-001",
				Codec:     "h264",
				Bitrate:   "2M",
				StartTime: startTime,
				WebRTCURL: "http://localhost:8889/test-stream",
				RTSPURL:   "rtsp://localhost:8554/test-stream",
			},
			Action:    "created",
			Timestamp: "2025-01-27T10:30:00Z",
		}

		data, err := json.Marshal(event)
		if err != nil {
			t.Fatalf("Failed to marshal event: %v", err)
		}

		// Verify it unmarshals correctly
		var unmarshaled StreamCreatedEvent
		if err := json.Unmarshal(data, &unmarshaled); err != nil {
			t.Fatalf("Failed to unmarshal event: %v", err)
		}

		if unmarshaled.Stream.StreamID != event.Stream.StreamID {
			t.Errorf("JSON serialization changed stream_id")
		}
	})

	t.Run("StreamDeletedEvent", func(t *testing.T) {
		event := StreamDeletedEvent{
			StreamID:  "test-stream",
			Action:    "deleted",
			Timestamp: "2025-01-27T10:30:00Z",
		}

		data, err := json.Marshal(event)
		if err != nil {
			t.Fatalf("Failed to marshal event: %v", err)
		}

		// Verify it unmarshals correctly
		var unmarshaled StreamDeletedEvent
		if err := json.Unmarshal(data, &unmarshaled); err != nil {
			t.Fatalf("Failed to unmarshal event: %v", err)
		}

		if unmarshaled.StreamID != event.StreamID {
			t.Errorf("JSON serialization changed stream_id")
		}
	})

	t.Run("StreamUpdatedEvent", func(t *testing.T) {
		startTime, _ := time.Parse(time.RFC3339, "2025-01-27T10:30:00Z")
		event := StreamUpdatedEvent{
			Stream: models.StreamData{
				StreamID:  "test-stream",
				DeviceID:  "device-001",
				Codec:     "h265",
				Bitrate:   "4M",
				StartTime: startTime,
				WebRTCURL: "http://localhost:8889/test-stream",
				RTSPURL:   "rtsp://localhost:8554/test-stream",
			},
			Action:    "updated",
			Timestamp: "2025-01-27T10:30:00Z",
		}

		data, err := json.Marshal(event)
		if err != nil {
			t.Fatalf("Failed to marshal event: %v", err)
		}

		// Verify it unmarshals correctly
		var unmarshaled StreamUpdatedEvent
		if err := json.Unmarshal(data, &unmarshaled); err != nil {
			t.Fatalf("Failed to unmarshal event: %v", err)
		}

		if unmarshaled.Stream.StreamID != event.Stream.StreamID {
			t.Errorf("JSON serialization changed stream_id")
		}
		if unmarshaled.Action != "updated" {
			t.Errorf("JSON serialization changed action")
		}
	})
}

func TestEventBroadcaster_BlockedChannels(t *testing.T) {
	broadcaster := &EventBroadcaster{
		channels: make([]chan<- interface{}, 0),
	}

	// Create a blocking channel (no buffer)
	blockingCh := make(chan interface{})

	// Create a non-blocking channel
	nonBlockingCh := make(chan interface{}, 1)

	// Subscribe both
	broadcaster.Subscribe(blockingCh)
	broadcaster.Subscribe(nonBlockingCh)

	// Broadcast event - should not block due to the first channel
	testEvent := "test_event"

	// This should complete quickly despite blocking channel
	done := make(chan bool, 1)
	go func() {
		broadcaster.Broadcast(testEvent)
		done <- true
	}()

	select {
	case <-done:
		// Good - broadcast completed
	case <-time.After(100 * time.Millisecond):
		t.Error("Broadcast was blocked by slow consumer")
	}

	// Verify non-blocking channel received event
	select {
	case event := <-nonBlockingCh:
		if event != testEvent {
			t.Errorf("Expected %v, got %v", testEvent, event)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Non-blocking channel did not receive event")
	}
}
