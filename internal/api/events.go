package api

import (
	"context"
	"net/http"
	"sync"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/sse"
	"github.com/smazurov/videonode/internal/api/models"
	"github.com/smazurov/videonode/internal/obs/exporters"
)

// SSE Event Types for Huma v2 native SSE

// CaptureSuccessEvent represents a successful screenshot capture
type CaptureSuccessEvent struct {
	DevicePath string `json:"device_path" example:"/dev/video0" doc:"Path to the video device"`
	Message    string `json:"message" example:"Screenshot captured successfully" doc:"Success message"`
	ImageData  string `json:"image_data" doc:"Base64-encoded screenshot image"`
	Timestamp  string `json:"timestamp" example:"2025-01-27T10:30:00Z" doc:"Capture timestamp"`
}

// CaptureErrorEvent represents a failed screenshot capture
type CaptureErrorEvent struct {
	DevicePath string `json:"device_path" example:"/dev/video0" doc:"Path to the video device"`
	Message    string `json:"message" example:"Screenshot capture failed" doc:"Error message"`
	Error      string `json:"error" example:"Device not found" doc:"Detailed error description"`
	Timestamp  string `json:"timestamp" example:"2025-01-27T10:30:00Z" doc:"Error timestamp"`
}

// DeviceDiscoveryEvent represents device hotplug events
type DeviceDiscoveryEvent struct {
	models.DeviceInfo
	Action    string `json:"action" example:"added" doc:"Action type: added, removed, changed"`
	Timestamp string `json:"timestamp" example:"2025-01-27T10:30:00Z" doc:"Event timestamp"`
}

// StreamCreatedEvent represents a successful stream creation
type StreamCreatedEvent struct {
	Stream    models.StreamData `json:"stream" doc:"Created stream data"`
	Action    string            `json:"action" example:"created" doc:"Action type"`
	Timestamp string            `json:"timestamp" example:"2025-01-27T10:30:00Z" doc:"Event timestamp"`
}

// StreamDeletedEvent represents a successful stream deletion
type StreamDeletedEvent struct {
	StreamID  string `json:"stream_id" example:"stream-001" doc:"Deleted stream identifier"`
	Action    string `json:"action" example:"deleted" doc:"Action type"`
	Timestamp string `json:"timestamp" example:"2025-01-27T10:30:00Z" doc:"Event timestamp"`
}

// Event broadcaster for inter-handler communication
type EventBroadcaster struct {
	channels []chan<- interface{}
	mutex    sync.RWMutex
}

var globalEventBroadcaster = &EventBroadcaster{
	channels: make([]chan<- interface{}, 0),
}

// Subscribe adds a channel to receive events
func (eb *EventBroadcaster) Subscribe(ch chan<- interface{}) {
	eb.mutex.Lock()
	defer eb.mutex.Unlock()
	eb.channels = append(eb.channels, ch)
}

// Unsubscribe removes a channel from receiving events
func (eb *EventBroadcaster) Unsubscribe(ch chan<- interface{}) {
	eb.mutex.Lock()
	defer eb.mutex.Unlock()
	for i, channel := range eb.channels {
		if channel == ch {
			eb.channels = append(eb.channels[:i], eb.channels[i+1:]...)
			break
		}
	}
}

// Broadcast sends an event to all subscribed channels
func (eb *EventBroadcaster) Broadcast(event interface{}) {
	eb.mutex.RLock()
	defer eb.mutex.RUnlock()
	for _, ch := range eb.channels {
		select {
		case ch <- event:
		default:
			// Skip if channel is full/blocked
		}
	}
}

// BroadcastCaptureSuccess sends a capture success event
func BroadcastCaptureSuccess(devicePath, imageData, timestamp string) {
	event := CaptureSuccessEvent{
		DevicePath: devicePath,
		Message:    "Screenshot captured successfully",
		ImageData:  imageData,
		Timestamp:  timestamp,
	}
	globalEventBroadcaster.Broadcast(event)
}

// BroadcastCaptureError sends a capture error event
func BroadcastCaptureError(devicePath, errorMsg, timestamp string) {
	event := CaptureErrorEvent{
		DevicePath: devicePath,
		Message:    "Screenshot capture failed",
		Error:      errorMsg,
		Timestamp:  timestamp,
	}
	globalEventBroadcaster.Broadcast(event)
}

// BroadcastDeviceDiscovery sends a device discovery event
func BroadcastDeviceDiscovery(action string, device models.DeviceInfo, timestamp string) {
	event := DeviceDiscoveryEvent{
		DeviceInfo: device,
		Action:     action,
		Timestamp:  timestamp,
	}
	globalEventBroadcaster.Broadcast(event)
}

// BroadcastStreamCreated sends a stream created event
func BroadcastStreamCreated(stream models.StreamData, timestamp string) {
	event := StreamCreatedEvent{
		Stream:    stream,
		Action:    "created",
		Timestamp: timestamp,
	}
	globalEventBroadcaster.Broadcast(event)
}

// BroadcastStreamDeleted sends a stream deleted event
func BroadcastStreamDeleted(streamID, timestamp string) {
	event := StreamDeletedEvent{
		StreamID:  streamID,
		Action:    "deleted",
		Timestamp: timestamp,
	}
	globalEventBroadcaster.Broadcast(event)
}

// registerSSERoutes registers the native Huma SSE endpoint
func (s *Server) registerSSERoutes() {
	// Register SSE endpoint with event type mapping
	sse.Register(s.api, huma.Operation{
		OperationID: "events-stream",
		Method:      http.MethodGet,
		Path:        "/api/events",
		Summary:     "Server-Sent Events Stream",
		Description: "Real-time event stream for capture results, device changes, and system status",
		Tags:        []string{"events"},
		Security:    withAuth(),
		Errors:      []int{401},
	}, func() map[string]any {
		// Application events
		eventTypes := map[string]any{
			"capture-success":  CaptureSuccessEvent{},
			"capture-error":    CaptureErrorEvent{},
			"device-discovery": DeviceDiscoveryEvent{},
			"stream-created":   StreamCreatedEvent{},
			"stream-deleted":   StreamDeletedEvent{},
		}

		// Add OBS events for this endpoint
		for eventName, eventType := range exporters.GetEventTypesForEndpoint("events") {
			eventTypes[eventName] = eventType
		}

		return eventTypes
	}(), func(ctx context.Context, input *struct{}, send sse.Sender) {
		// Create event channel for this connection
		eventCh := make(chan interface{}, 10)

		// Subscribe to global event broadcaster
		globalEventBroadcaster.Subscribe(eventCh)
		defer globalEventBroadcaster.Unsubscribe(eventCh)

		// Keep connection alive and forward events
		for {
			select {
			case <-ctx.Done():
				return
			case event := <-eventCh:
				// Send event using Huma's SSE sender
				send.Data(event)
			}
		}
	})
}
