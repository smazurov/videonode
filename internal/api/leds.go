package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
)

// LED request/response models

// LEDRequest represents a request to control an LED
type LEDRequest struct {
	Body struct {
		Type    string  `json:"type" example:"user" doc:"LED type (board-specific: user, system, blue, green, etc.)"`
		Enabled bool    `json:"enabled" example:"true" doc:"Whether the LED should be on or off"`
		Pattern *string `json:"pattern,omitempty" example:"solid" doc:"Optional LED pattern (solid, blink, heartbeat)"`
	}
}

// LEDCapabilitiesResponse represents the LED capabilities of the current board
type LEDCapabilitiesResponse struct {
	Body struct {
		AvailableTypes    []string `json:"available_types" doc:"List of available LED types on this board"`
		AvailablePatterns []string `json:"available_patterns" doc:"List of available LED patterns on this board"`
	}
}

// registerLEDRoutes registers LED control endpoints
func (s *Server) registerLEDRoutes() {
	// Only register if LED controller is available
	if s.options.LEDController == nil {
		s.logger.Debug("LED controller not available, skipping LED routes")
		return
	}

	// Control LED
	huma.Register(s.api, huma.Operation{
		OperationID: "control-led",
		Method:      http.MethodPost,
		Path:        "/api/leds",
		Summary:     "Control LED",
		Description: "Control an LED's state and optional pattern. LED types and patterns are board-specific.",
		Tags:        []string{"leds"},
		Errors:      []int{400, 401, 500},
		Security:    withAuth(),
	}, func(ctx context.Context, input *LEDRequest) (*struct{}, error) {
		pattern := ""
		if input.Body.Pattern != nil {
			pattern = *input.Body.Pattern
		}

		if err := s.options.LEDController.Set(input.Body.Type, input.Body.Enabled, pattern); err != nil {
			return nil, huma.Error400BadRequest("Failed to control LED", err)
		}

		return &struct{}{}, nil
	})

	// Get LED capabilities
	huma.Register(s.api, huma.Operation{
		OperationID: "get-led-capabilities",
		Method:      http.MethodGet,
		Path:        "/api/leds/capabilities",
		Summary:     "Get LED Capabilities",
		Description: "Get the list of available LED types and patterns for this board",
		Tags:        []string{"leds"},
		Errors:      []int{401},
		Security:    withAuth(),
	}, func(ctx context.Context, input *struct{}) (*LEDCapabilitiesResponse, error) {
		return &LEDCapabilitiesResponse{
			Body: struct {
				AvailableTypes    []string `json:"available_types" doc:"List of available LED types on this board"`
				AvailablePatterns []string `json:"available_patterns" doc:"List of available LED patterns on this board"`
			}{
				AvailableTypes:    s.options.LEDController.Available(),
				AvailablePatterns: s.options.LEDController.Patterns(),
			},
		}, nil
	})

	s.logger.Info("LED routes registered")
}