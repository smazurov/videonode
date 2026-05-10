package events

import (
	"github.com/kelindar/event"
)

// Bus wraps kelindar/event dispatcher for event broadcasting.
type Bus struct {
	dispatcher *event.Dispatcher
}

// New creates a new event bus.
func New() *Bus {
	return &Bus{
		dispatcher: event.NewDispatcher(),
	}
}

// Publish publishes an event to all subscribers.
func (b *Bus) Publish(ev Event) {
	switch e := ev.(type) {
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
	case StreamMetricsEvent:
		event.Publish(b.dispatcher, e)
	case LogEntryEvent:
		event.Publish(b.dispatcher, e)
	case StreamCrashedEvent:
		event.Publish(b.dispatcher, e)
	}
}

// Subscribe subscribes to events with a handler function.
// The handler type determines which events it receives (type inference).
// Returns an unsubscribe function.
func (b *Bus) Subscribe(handler any) func() {
	switch h := handler.(type) {
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
	case func(StreamMetricsEvent):
		return event.Subscribe(b.dispatcher, h)
	case func(LogEntryEvent):
		return event.Subscribe(b.dispatcher, h)
	case func(StreamCrashedEvent):
		return event.Subscribe(b.dispatcher, h)
	default:
		// Return a no-op function if handler type is not recognized
		return func() {}
	}
}
