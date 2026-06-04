package events

// SubscribeToChannel bridges a typed kelindar/event subscription to a channel.
// Needed for SSE integration where Huma expects a channel-based select loop.
// The send is non-blocking: if the channel is full the event is dropped so a
// slow consumer can never stall the dispatcher.
func SubscribeToChannel[T Event](bus *Bus, ch chan<- any) func() {
	return Subscribe(bus, func(e T) {
		select {
		case ch <- e:
		default:
			// Drop event if channel is full (non-blocking)
		}
	})
}
