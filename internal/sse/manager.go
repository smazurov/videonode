package sse

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/smazurov/videonode/v4l2_detector"
	"github.com/tmaxmax/go-sse"
)

// EventType defines the type of SSE event
type EventType string

const (
	EventDeviceDiscovery EventType = "device-discovery"
	EventEncoderUpdate   EventType = "encoder-update"
	EventSystemStatus    EventType = "system-status"
	EventError           EventType = "error"
	EventCaptureSuccess  EventType = "capture-success"
	EventCaptureError    EventType = "capture-error"
	EventMetricsUpdate   EventType = "metrics-update"
)

// DeviceResponse represents device information response
type DeviceResponse struct {
	Devices []v4l2_detector.DeviceInfo `json:"devices"`
	Count   int                        `json:"count"`
}

// Manager manages the SSE server and related functionality
type Manager struct {
	server *sse.Server
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// Monitoring
	deviceMonitorCancel context.CancelFunc
	udevMonitorCancel   context.CancelFunc

	// State tracking
	lastDevices      DeviceResponse
	lastDevicesMutex sync.RWMutex
	eventCounter     atomic.Int64

	// Configuration
	defaultChannel  string
	pollingInterval time.Duration

	// Dependencies
	getDevicesData func() (DeviceResponse, error)
}

// Config holds configuration for the SSE manager
type Config struct {
	DefaultChannel  string
	PollingInterval time.Duration
}

// DefaultConfig returns a default configuration
func DefaultConfig() Config {
	return Config{
		DefaultChannel:  "updates",
		PollingInterval: 10 * time.Second,
	}
}

// New creates a new SSE manager
func New(config Config, getDevicesFunc func() (DeviceResponse, error)) *Manager {
	ctx, cancel := context.WithCancel(context.Background())

	manager := &Manager{
		server:          &sse.Server{},
		ctx:             ctx,
		cancel:          cancel,
		defaultChannel:  config.DefaultChannel,
		pollingInterval: config.PollingInterval,
		getDevicesData:  getDevicesFunc,
	}

	// Configure the OnSession callback
	manager.server.OnSession = manager.handleNewSession

	return manager
}

// handleNewSession handles new SSE client connections
func (m *Manager) handleNewSession(w http.ResponseWriter, r *http.Request) (topics []string, ok bool) {
	log.Printf("SSE: New session request from %s for path %s", r.RemoteAddr, r.URL.Path)

	return []string{m.defaultChannel}, true
}

// Start initializes and starts all SSE components
func (m *Manager) Start() error {
	log.Println("SSE: Starting SSE manager")

	// Try to start udev monitoring first, fall back to polling if it fails
	if err := m.startUdevMonitoring(); err != nil {
		log.Printf("SSE: Failed to start udev monitoring: %v. Using polling instead.", err)
		m.startPollingMonitor()
	}

	return nil
}

// Shutdown gracefully shuts down all SSE components
func (m *Manager) Shutdown(timeout time.Duration) error {
	log.Println("SSE: Shutting down SSE manager")

	// Cancel the main context
	m.cancel()

	// Cancel specific monitors if they're running
	if m.deviceMonitorCancel != nil {
		m.deviceMonitorCancel()
	}
	if m.udevMonitorCancel != nil {
		m.udevMonitorCancel()
	}

	// Wait for goroutines with timeout
	done := make(chan struct{})
	go func() {
		m.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		log.Println("SSE: All goroutines stopped")
	case <-time.After(timeout):
		log.Println("SSE: Shutdown timeout exceeded")
	}

	// Shutdown the SSE server
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	if err := m.server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("error shutting down SSE server: %w", err)
	}

	log.Println("SSE: Manager shutdown complete")
	return nil
}

// BroadcastEvent sends an event to all subscribed sessions
func (m *Manager) BroadcastEvent(eventType EventType, data interface{}) error {
	if m.server == nil {
		return fmt.Errorf("SSE server not initialized")
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		log.Printf("SSE: Error marshaling event data for type %s: %v", eventType, err)
		return fmt.Errorf("error marshaling event data: %w", err)
	}

	// Generate event ID
	eventID := m.eventCounter.Add(1)

	msg := &sse.Message{}
	msg.Type = sse.Type(string(eventType))
	msg.ID = sse.ID(fmt.Sprintf("%d", eventID))
	msg.AppendData(string(jsonData))

	err = m.server.Publish(msg, m.defaultChannel)
	if err != nil {
		log.Printf("SSE: Error publishing event '%s' to topic '%s': %v", eventType, m.defaultChannel, err)
		return err
	}

	return nil
}

// BroadcastCustomEvent sends a simple SSE event with a custom event name
func (m *Manager) BroadcastCustomEvent(eventName string, data interface{}) error {
	if m.server == nil {
		return fmt.Errorf("SSE server not initialized")
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		log.Printf("SSE: Error marshaling event data for event %s: %v", eventName, err)
		return fmt.Errorf("error marshaling event data: %w", err)
	}

	// Generate event ID
	eventID := m.eventCounter.Add(1)

	msg := &sse.Message{}
	msg.Type = sse.Type(eventName)
	msg.ID = sse.ID(fmt.Sprintf("%d", eventID))
	msg.AppendData(string(jsonData))

	err = m.server.Publish(msg, m.defaultChannel)
	if err != nil {
		log.Printf("SSE: Error publishing event '%s' to topic '%s': %v", eventName, m.defaultChannel, err)
		return err
	}

	return nil
}

// GetHandler returns the HTTP handler for SSE
func (m *Manager) GetHandler() http.HandlerFunc {
	return m.server.ServeHTTP
}
