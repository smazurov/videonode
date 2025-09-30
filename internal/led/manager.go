package led

import (
	"log/slog"
	"sync"
)

// EventBroadcaster is the interface for subscribing to system events
type EventBroadcaster interface {
	Subscribe(ch chan<- interface{})
	Unsubscribe(ch chan<- interface{})
}

// StreamStateEvent represents a change in stream enabled state
// This will be defined in internal/api/events.go
type StreamStateEvent interface {
	GetStreamID() string
	IsEnabled() bool
}

// Manager subscribes to stream events and controls system LED based on aggregate state
type Manager struct {
	controller      Controller
	broadcaster     EventBroadcaster
	eventChan       chan interface{}
	stopChan        chan struct{}
	logger          *slog.Logger
	streamStates    map[string]bool // streamID -> enabled state
	streamStatesMux sync.RWMutex
}

// NewManager creates a new LED manager that reacts to stream state changes
func NewManager(controller Controller, broadcaster EventBroadcaster, logger *slog.Logger) *Manager {
	return &Manager{
		controller:   controller,
		broadcaster:  broadcaster,
		eventChan:    make(chan interface{}, 10),
		stopChan:     make(chan struct{}),
		logger:       logger,
		streamStates: make(map[string]bool),
	}
}

// Start begins listening for stream state change events
func (m *Manager) Start() {
	m.broadcaster.Subscribe(m.eventChan)
	go m.eventLoop()
	m.logger.Info("LED manager started")
}

// Stop stops the LED manager and unsubscribes from events
func (m *Manager) Stop() {
	close(m.stopChan)
	m.broadcaster.Unsubscribe(m.eventChan)
	m.logger.Info("LED manager stopped")
}

// eventLoop processes incoming stream state events
func (m *Manager) eventLoop() {
	for {
		select {
		case <-m.stopChan:
			return
		case event := <-m.eventChan:
			m.handleEvent(event)
		}
	}
}

// handleEvent processes a single event
func (m *Manager) handleEvent(event interface{}) {
	// Type assert to stream state event
	stateEvent, ok := event.(StreamStateEvent)
	if !ok {
		return // Not a stream state event, ignore
	}

	streamID := stateEvent.GetStreamID()
	enabled := stateEvent.IsEnabled()

	m.streamStatesMux.Lock()
	m.streamStates[streamID] = enabled
	m.streamStatesMux.Unlock()

	m.logger.Debug("Stream state changed",
		"stream_id", streamID,
		"enabled", enabled)

	// Update system LED based on aggregate state
	m.updateSystemLED()
}

// updateSystemLED sets the system LED pattern based on whether all streams are enabled
func (m *Manager) updateSystemLED() {
	m.streamStatesMux.RLock()
	defer m.streamStatesMux.RUnlock()

	// Check if we have any streams
	if len(m.streamStates) == 0 {
		// No streams, use blinking pattern
		if err := m.controller.Set("system", true, "blink"); err != nil {
			m.logger.Warn("Failed to set system LED to blink", "error", err)
		}
		return
	}

	// Check if all streams are enabled
	allEnabled := true
	for _, enabled := range m.streamStates {
		if !enabled {
			allEnabled = false
			break
		}
	}

	// Set LED pattern based on state
	if allEnabled {
		// All streams enabled - solid LED
		if err := m.controller.Set("system", true, "solid"); err != nil {
			m.logger.Warn("Failed to set system LED to solid", "error", err)
		}
		m.logger.Debug("All streams enabled, system LED set to solid")
	} else {
		// Some streams disabled - blinking LED
		if err := m.controller.Set("system", true, "blink"); err != nil {
			m.logger.Warn("Failed to set system LED to blink", "error", err)
		}
		m.logger.Debug("Not all streams enabled, system LED set to blink")
	}
}

// GetController returns the underlying LED controller for direct API access
func (m *Manager) GetController() Controller {
	return m.controller
}