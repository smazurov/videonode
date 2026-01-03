package events

import "github.com/kelindar/event"

// SubscribeToChannel bridges kelindar/event callback-based subscriptions to channels
// This is needed for SSE integration where Huma expects a channel-based select loop.
func SubscribeToChannel[T Event](bus *Bus, ch chan<- any) func() {
	return event.Subscribe(bus.dispatcher, func(e T) {
		select {
		case ch <- e:
		default:
			// Drop event if channel is full (non-blocking)
		}
	})
}
