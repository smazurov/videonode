package exporters

import (
	"sync"

	"github.com/smazurov/videonode/internal/events"
)

// MockEventBus for testing
type MockEventBus struct {
	mu     sync.Mutex
	events []any
}

func (m *MockEventBus) Publish(ev events.Event) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, ev)
}

func (m *MockEventBus) Subscribe(handler any) func() {
	return func() {}
}

func (m *MockEventBus) GetEvents() []any {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]any, len(m.events))
	copy(result, m.events)
	return result
}

func (m *MockEventBus) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = nil
}
