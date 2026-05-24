package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/smazurov/videonode/internal/api/models"
)

// registerComposerRoutes wires the v2 /api/composers CRUD surface. Handlers
// are stubs that emit the OpenAPI schema; B6 replaces them with real
// service-backed implementations.
func (s *Server) registerComposerRoutes() {
	huma.Register(s.api, huma.Operation{
		OperationID: "list-composers",
		Method:      http.MethodGet,
		Path:        "/api/composers",
		Summary:     "List Composers",
		Description: "List all configured composers (GLES BGRA compositors).",
		Tags:        []string{"composers"},
		Errors:      []int{401, 500},
		Security:    withAuth(),
	}, func(_ context.Context, _ *struct{}) (*models.ComposerListResponse, error) {
		return &models.ComposerListResponse{Body: models.ComposerListData{Composers: []models.ComposerData{}, Count: 0}}, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "create-composer",
		Method:      http.MethodPost,
		Path:        "/api/composers",
		Summary:     "Create Composer",
		Description: "Create a new composer.",
		Tags:        []string{"composers"},
		Errors:      []int{400, 401, 409, 500},
		Security:    withAuth(),
	}, func(_ context.Context, input *models.ComposerRequest) (*models.ComposerResponse, error) {
		return &models.ComposerResponse{Body: models.ComposerData{
			ID:     input.Body.ID,
			Canvas: input.Body.Canvas,
			Inputs: input.Body.Inputs,
			Layout: input.Body.Layout,
		}}, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "get-composer",
		Method:      http.MethodGet,
		Path:        "/api/composers/{composer_id}",
		Summary:     "Get Composer",
		Description: "Get a single composer by id.",
		Tags:        []string{"composers"},
		Errors:      []int{401, 404, 500},
		Security:    withAuth(),
	}, func(_ context.Context, input *struct {
		ComposerID string `path:"composer_id" example:"main-scene" doc:"Composer identifier"`
	},
	) (*models.ComposerResponse, error) {
		return &models.ComposerResponse{Body: models.ComposerData{ID: input.ComposerID}}, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "update-composer",
		Method:      http.MethodPatch,
		Path:        "/api/composers/{composer_id}",
		Summary:     "Update Composer",
		Description: "Patch fields on an existing composer.",
		Tags:        []string{"composers"},
		Errors:      []int{400, 401, 404, 500},
		Security:    withAuth(),
	}, func(_ context.Context, input *struct {
		ComposerID string `path:"composer_id" example:"main-scene" doc:"Composer identifier"`
		Body       models.ComposerUpdateRequestData
	},
	) (*models.ComposerResponse, error) {
		return &models.ComposerResponse{Body: models.ComposerData{ID: input.ComposerID}}, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "delete-composer",
		Method:      http.MethodDelete,
		Path:        "/api/composers/{composer_id}",
		Summary:     "Delete Composer",
		Description: "Delete a composer. Fails if any stream references it.",
		Tags:        []string{"composers"},
		Errors:      []int{401, 404, 409, 500},
		Security:    withAuth(),
	}, func(_ context.Context, _ *struct {
		ComposerID string `path:"composer_id" example:"main-scene" doc:"Composer identifier"`
	},
	) (*struct{}, error) {
		return &struct{}{}, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "update-composer-layout",
		Method:      http.MethodPatch,
		Path:        "/api/composers/{composer_id}/layout",
		Summary:     "Update Composer Layout",
		Description: "Replace the composer's layout slot rectangles.",
		Tags:        []string{"composers"},
		Errors:      []int{400, 401, 404, 500},
		Security:    withAuth(),
	}, func(_ context.Context, input *struct {
		ComposerID string `path:"composer_id" example:"main-scene" doc:"Composer identifier"`
		Body       struct {
			Layout []models.LayoutSlotData `json:"layout" doc:"Replacement layout slot rectangles"`
		}
	},
	) (*models.ComposerResponse, error) {
		return &models.ComposerResponse{Body: models.ComposerData{ID: input.ComposerID, Layout: input.Body.Layout}}, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "update-composer-input-effect",
		Method:      http.MethodPatch,
		Path:        "/api/composers/{composer_id}/inputs/{input_ref}/effect",
		Summary:     "Update Composer Input Effect",
		Description: "Replace the effect applied to a single composer input.",
		Tags:        []string{"composers"},
		Errors:      []int{400, 401, 404, 500},
		Security:    withAuth(),
	}, func(_ context.Context, input *struct {
		ComposerID string `path:"composer_id" example:"main-scene" doc:"Composer identifier"`
		InputRef   string `path:"input_ref" example:"source:cam-host" doc:"Composer input ref"`
		Body       struct {
			Effect *models.EffectData `json:"effect,omitempty" doc:"Effect to apply, or null to clear"`
		}
	},
	) (*models.ComposerResponse, error) {
		_ = input.InputRef
		return &models.ComposerResponse{Body: models.ComposerData{ID: input.ComposerID}}, nil
	})
}
