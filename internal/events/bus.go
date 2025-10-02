package events

import (
	"github.com/kelindar/event"
)

// Bus wraps kelindar/event dispatcher for event broadcasting
type Bus struct {
	dispatcher *event.Dispatcher
}

// New creates a new event bus
func New() *Bus {
	return &Bus{
		dispatcher: event.NewDispatcher(),
	}
}

// Publish publishes an event to all subscribers
// Usage: bus.Publish(CaptureSuccessEvent{...})
func (b *Bus) Publish(ev Event) {
	// Use type switch to call the generic Publish with the correct type
	switch e := ev.(type) {
	case CaptureSuccessEvent:
		event.Publish(b.dispatcher, e)
	case CaptureErrorEvent:
		event.Publish(b.dispatcher, e)
	case DeviceDiscoveryEvent:
		event.Publish(b.dispatcher, e)
	case StreamCreatedEvent:
		event.Publish(b.dispatcher, e)
	case StreamUpdatedEvent:
		event.Publish(b.dispatcher, e)
	case StreamDeletedEvent:
		event.Publish(b.dispatcher, e)
	case StreamStateChangedEvent:
		event.Publish(b.dispatcher, e)
	case MediaMTXMetricsEvent:
		event.Publish(b.dispatcher, e)
	case OBSAlertEvent:
		event.Publish(b.dispatcher, e)
	case StreamMetricsEvent:
		event.Publish(b.dispatcher, e)
	}
}

// Subscribe subscribes to events with a handler function
// The handler type determines which events it receives (type inference)
// Returns an unsubscribe function
// Usage: unsub := bus.Subscribe(func(e CaptureSuccessEvent) { ... })
func (b *Bus) Subscribe(handler any) func() {
	// This is a bit tricky - we need to extract the type from the handler
	// The kelindar/event library uses reflection to determine the event type
	// We'll use a type assertion approach

	// For each known event type, check if the handler matches
	switch h := handler.(type) {
	case func(CaptureSuccessEvent):
		return event.Subscribe(b.dispatcher, h)
	case func(CaptureErrorEvent):
		return event.Subscribe(b.dispatcher, h)
	case func(DeviceDiscoveryEvent):
		return event.Subscribe(b.dispatcher, h)
	case func(StreamCreatedEvent):
		return event.Subscribe(b.dispatcher, h)
	case func(StreamUpdatedEvent):
		return event.Subscribe(b.dispatcher, h)
	case func(StreamDeletedEvent):
		return event.Subscribe(b.dispatcher, h)
	case func(StreamStateChangedEvent):
		return event.Subscribe(b.dispatcher, h)
	case func(MediaMTXMetricsEvent):
		return event.Subscribe(b.dispatcher, h)
	case func(OBSAlertEvent):
		return event.Subscribe(b.dispatcher, h)
	case func(StreamMetricsEvent):
		return event.Subscribe(b.dispatcher, h)
	default:
		// Return a no-op function if handler type is not recognized
		return func() {}
	}
}