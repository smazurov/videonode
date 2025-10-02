package led

import (
	"log/slog"
	"sync"

	"github.com/smazurov/videonode/internal/events"
)

// Manager subscribes to stream events and controls system LED based on aggregate state
type Manager struct {
	controller      Controller
	eventBus        *events.Bus
	unsubscribe     func()
	stopChan        chan struct{}
	logger          *slog.Logger
	streamStates    map[string]bool // streamID -> enabled state
	streamStatesMux sync.RWMutex
}

// NewManager creates a new LED manager that reacts to stream state changes
func NewManager(controller Controller, eventBus *events.Bus, logger *slog.Logger) *Manager {
	return &Manager{
		controller:   controller,
		eventBus:     eventBus,
		stopChan:     make(chan struct{}),
		logger:       logger,
		streamStates: make(map[string]bool),
	}
}

// Start begins listening for stream state change events
func (m *Manager) Start() {
	// Subscribe to stream state changed events
	m.unsubscribe = m.eventBus.Subscribe(func(e events.StreamStateChangedEvent) {
		m.handleEvent(e)
	})
	m.logger.Info("LED manager started")
}

// Stop stops the LED manager and unsubscribes from events
func (m *Manager) Stop() {
	if m.unsubscribe != nil {
		m.unsubscribe()
	}
	close(m.stopChan)
	m.logger.Info("LED manager stopped")
}

// handleEvent processes a single stream state changed event
func (m *Manager) handleEvent(event events.StreamStateChangedEvent) {
	streamID := event.GetStreamID()
	enabled := event.IsEnabled()

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