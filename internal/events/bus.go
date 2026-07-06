package events

import (
	"fmt"
	"reflect"
	"sync"
)

// Bus is a type-keyed in-process pub/sub with zero idle cost: no goroutines,
// no timers. Publish fans out synchronously on the publisher's goroutine, so
// handlers must never block (ours are drop-on-full channel sends). No bus
// lock is held while handlers run, so a handler may itself publish — the
// logging callback publishes LogEntryEvent, and a handler that logs re-enters
// Publish.
type Bus struct {
	mu    sync.RWMutex
	subs  map[uint32][]*subscription
	kinds map[uint32]reflect.Type
}

type subscription struct {
	handler any // func(T)
}

// New creates a new event bus.
func New() *Bus {
	return &Bus{
		subs:  make(map[uint32][]*subscription),
		kinds: make(map[uint32]reflect.Type),
	}
}

// Publish broadcasts a typed event to every subscriber registered for that
// event type. The concrete type T determines the delivery topic, so adding a
// new event type requires no change here — just a type implementing Event.
// Handlers run synchronously on the caller's goroutine.
func Publish[T Event](b *Bus, ev T) {
	b.mu.RLock()
	// Subscribe/unsubscribe replace the slice instead of mutating it, so the
	// snapshot stays valid (and Publish allocation-free) after unlocking.
	list := b.subs[ev.Type()]
	b.mu.RUnlock()
	for _, s := range list {
		s.handler.(func(T))(ev)
	}
}

// Subscribe registers a handler for events of type T and returns an
// idempotent unsubscribe function. The handler's parameter type selects which
// events it receives. Panics if T's Type() code is already registered by a
// different concrete type; codes are hand-assigned in types.go and must stay
// unique. T must implement Type() with a value receiver.
func Subscribe[T Event](b *Bus, handler func(T)) func() {
	var zero T
	code := zero.Type()
	kind := reflect.TypeFor[T]()

	b.mu.Lock()
	if prev, ok := b.kinds[code]; ok && prev != kind {
		b.mu.Unlock()
		panic(fmt.Sprintf("events: duplicate Type() code %d: %v vs %v", code, prev, kind))
	}
	b.kinds[code] = kind
	s := &subscription{handler: handler}
	b.subs[code] = append(append([]*subscription(nil), b.subs[code]...), s)
	b.mu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() {
			b.mu.Lock()
			old := b.subs[code]
			next := make([]*subscription, 0, len(old))
			for _, e := range old {
				if e != s {
					next = append(next, e)
				}
			}
			b.subs[code] = next
			b.mu.Unlock()
		})
	}
}
