package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/smazurov/videonode/internal/api/models"
	"github.com/smazurov/videonode/internal/ffmpeg"
)

// registerOptionsRoutes registers all FFmpeg options-related API routes.
func (s *Server) registerOptionsRoutes() {
	// Get available FFmpeg options
	huma.Register(s.api, huma.Operation{
		OperationID: "get-ffmpeg-options",
		Method:      http.MethodGet,
		Path:        "/api/options",
		Summary:     "Get FFmpeg Options",
		Description: "Get all available FFmpeg options with metadata including descriptions, categories, and conflict information",
		Tags:        []string{"configuration"},
		Security:    withAuth(),
		Errors:      []int{401, 500},
	}, func(_ context.Context, _ *struct{}) (*models.OptionsResponse, error) {
		return &models.OptionsResponse{
			Body: models.OptionsData{
				Options: ffmpeg.AllOptions,
			},
		}, nil
	})
}
