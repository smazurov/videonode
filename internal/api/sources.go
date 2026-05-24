package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/smazurov/videonode/internal/api/models"
)

// registerSourceRoutes wires the v2 /api/sources CRUD surface. The handlers
// are stubs that emit the OpenAPI schema; B5 replaces them with real
// service-backed implementations.
func (s *Server) registerSourceRoutes() {
	huma.Register(s.api, huma.Operation{
		OperationID: "list-sources",
		Method:      http.MethodGet,
		Path:        "/api/sources",
		Summary:     "List Sources",
		Description: "List all configured sources (frame producers).",
		Tags:        []string{"sources"},
		Errors:      []int{401, 500},
		Security:    withAuth(),
	}, func(_ context.Context, _ *struct{}) (*models.SourceListResponse, error) {
		return &models.SourceListResponse{Body: models.SourceListData{Sources: []models.SourceData{}, Count: 0}}, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "create-source",
		Method:      http.MethodPost,
		Path:        "/api/sources",
		Summary:     "Create Source",
		Description: "Create a new source.",
		Tags:        []string{"sources"},
		Errors:      []int{400, 401, 409, 500},
		Security:    withAuth(),
	}, func(_ context.Context, input *models.SourceRequest) (*models.SourceResponse, error) {
		return &models.SourceResponse{Body: models.SourceData{
			ID:       input.Body.ID,
			Device:   input.Body.Device,
			TestMode: input.Body.TestMode,
		}}, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "get-source",
		Method:      http.MethodGet,
		Path:        "/api/sources/{source_id}",
		Summary:     "Get Source",
		Description: "Get a single source by id.",
		Tags:        []string{"sources"},
		Errors:      []int{401, 404, 500},
		Security:    withAuth(),
	}, func(_ context.Context, input *struct {
		SourceID string `path:"source_id" example:"hdmi-slides" doc:"Source identifier"`
	},
	) (*models.SourceResponse, error) {
		return &models.SourceResponse{Body: models.SourceData{ID: input.SourceID}}, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "update-source",
		Method:      http.MethodPatch,
		Path:        "/api/sources/{source_id}",
		Summary:     "Update Source",
		Description: "Patch fields on an existing source.",
		Tags:        []string{"sources"},
		Errors:      []int{400, 401, 404, 500},
		Security:    withAuth(),
	}, func(_ context.Context, input *struct {
		SourceID string `path:"source_id" example:"hdmi-slides" doc:"Source identifier"`
		Body     models.SourceUpdateRequestData
	},
	) (*models.SourceResponse, error) {
		return &models.SourceResponse{Body: models.SourceData{ID: input.SourceID}}, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "delete-source",
		Method:      http.MethodDelete,
		Path:        "/api/sources/{source_id}",
		Summary:     "Delete Source",
		Description: "Delete a source. Fails if any composer or stream references it.",
		Tags:        []string{"sources"},
		Errors:      []int{401, 404, 409, 500},
		Security:    withAuth(),
	}, func(_ context.Context, _ *struct {
		SourceID string `path:"source_id" example:"hdmi-slides" doc:"Source identifier"`
	},
	) (*struct{}, error) {
		return &struct{}{}, nil
	})
}
