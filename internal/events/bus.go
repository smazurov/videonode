package events

import (
	"github.com/kelindar/event"
)

// Bus wraps a kelindar/event dispatcher for in-process event broadcasting.
type Bus struct {
	dispatcher *event.Dispatcher
}

// New creates a new event bus.
func New() *Bus {
	return &Bus{
		dispatcher: event.NewDispatcher(),
	}
}

// Publish broadcasts a typed event to every subscriber registered for that
// event type. The concrete type T determines the delivery topic, so adding a
// new event type requires no change here — just a type implementing Event.
func Publish[T Event](b *Bus, ev T) {
	event.Publish(b.dispatcher, ev)
}

// Subscribe registers a handler for events of type T and returns an
// unsubscribe function. The handler's parameter type selects which events it
// receives.
func Subscribe[T Event](b *Bus, handler func(T)) func() {
	return event.Subscribe(b.dispatcher, handler)
}
