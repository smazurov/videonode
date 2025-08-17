package api

import (
	"github.com/smazurov/videonode/internal/obs/exporters"
)

// OBSSSEAdapter adapts the API's event broadcaster to work with the OBS SSE exporter
type OBSSSEAdapter struct{}

// NewOBSSSEAdapter creates a new adapter for OBS SSE integration
func NewOBSSSEAdapter() *OBSSSEAdapter {
	return &OBSSSEAdapter{}
}

// BroadcastEvent implements the SSEBroadcaster interface for the OBS exporter
func (a *OBSSSEAdapter) BroadcastEvent(eventType string, data interface{}) error {
	// The data is already a typed struct from the SSE exporter
	// Just broadcast it directly - Huma SSE will handle the typing
	globalEventBroadcaster.Broadcast(data)

	return nil
}

// GetSSEBroadcaster returns the SSE broadcaster for OBS integration
func (s *Server) GetSSEBroadcaster() exporters.SSEBroadcaster {
	if s.obsSSEAdapter == nil {
		s.obsSSEAdapter = NewOBSSSEAdapter()
	}
	return s.obsSSEAdapter
}
