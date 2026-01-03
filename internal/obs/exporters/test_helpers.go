package exporters

import (
	"sync"

	"github.com/smazurov/videonode/internal/events"
)

// MockEventBus for testing.
type MockEventBus struct {
	mu     sync.Mutex
	events []any
}

// Publish adds an event to the mock event bus.
func (m *MockEventBus) Publish(ev events.Event) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, ev)
}

// Subscribe returns a no-op unsubscribe function for testing.
func (m *MockEventBus) Subscribe(_ any) func() {
	return func() {}
}

// GetEvents returns all published events.
func (m *MockEventBus) GetEvents() []any {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]any, len(m.events))
	copy(result, m.events)
	return result
}

// Reset clears all events from the mock event bus.
func (m *MockEventBus) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = nil
}
