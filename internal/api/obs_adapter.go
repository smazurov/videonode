package api

import (
	"github.com/smazurov/videonode/internal/obs/exporters"
)

// OBSSSEAdapter adapts the API's event broadcaster to work with the OBS SSE exporter
type OBSSSEAdapter struct {
	routes       map[string]string            // eventType -> endpoint name
	broadcasters map[string]func(interface{}) // endpoint -> broadcaster function
}

// NewOBSSSEAdapter creates a new adapter for OBS SSE integration
func NewOBSSSEAdapter() *OBSSSEAdapter {
	return &OBSSSEAdapter{
		routes:       make(map[string]string),
		broadcasters: make(map[string]func(interface{})),
	}
}

// RegisterEndpoint registers a broadcaster for an endpoint
func (a *OBSSSEAdapter) RegisterEndpoint(endpoint string, broadcaster func(interface{})) {
	a.broadcasters[endpoint] = broadcaster
}

// Initialize sets up the routing based on OBS configuration
func (a *OBSSSEAdapter) Initialize() {
	// Register broadcasters
	a.RegisterEndpoint("events", globalEventBroadcaster.Broadcast)
	a.RegisterEndpoint("metrics", globalMetricsBroadcaster.Broadcast)

	// Set up routes from OBS configuration
	a.routes = exporters.GetEventRoutes()
}

// BroadcastEvent implements the SSEBroadcaster interface for the OBS exporter
func (a *OBSSSEAdapter) BroadcastEvent(eventType string, data interface{}) error {
	// Get the endpoint for this event type
	endpoint := a.routes[eventType]
	if endpoint == "" {
		endpoint = "events" // default to events endpoint
	}

	// Route to the appropriate broadcaster
	if broadcaster, exists := a.broadcasters[endpoint]; exists {
		broadcaster(data)
	}

	return nil
}

// GetSSEBroadcaster returns the SSE broadcaster for OBS integration
func (s *Server) GetSSEBroadcaster() exporters.SSEBroadcaster {
	if s.obsSSEAdapter == nil {
		s.obsSSEAdapter = NewOBSSSEAdapter()
		s.obsSSEAdapter.Initialize()
	}
	return s.obsSSEAdapter
}
