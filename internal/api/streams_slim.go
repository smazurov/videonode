package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/smazurov/videonode/internal/api/models"
)

// registerStreamSlimRoutes exposes the v2 StreamSlimData shape on a parallel
// path so the OpenAPI generator emits the schema. The legacy /api/streams
// surface stays untouched; B7 collapses the two once the slim shape is the
// only one.
func (s *Server) registerStreamSlimRoutes() {
	huma.Register(s.api, huma.Operation{
		OperationID: "list-streams-v2",
		Method:      http.MethodGet,
		Path:        "/api/v2/streams",
		Summary:     "List Streams (v2 slim)",
		Description: "List streams in the v2 slim shape (no device/canvas/layout/effects fields).",
		Tags:        []string{"streams"},
		Errors:      []int{401, 500},
		Security:    withAuth(),
	}, func(_ context.Context, _ *struct{}) (*models.StreamSlimListResponse, error) {
		return &models.StreamSlimListResponse{Body: models.StreamSlimListData{Streams: []models.StreamSlimData{}, Count: 0}}, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "get-stream-v2",
		Method:      http.MethodGet,
		Path:        "/api/v2/streams/{stream_id}",
		Summary:     "Get Stream (v2 slim)",
		Description: "Get a single stream in the v2 slim shape.",
		Tags:        []string{"streams"},
		Errors:      []int{401, 404, 500},
		Security:    withAuth(),
	}, func(_ context.Context, input *struct {
		StreamID string `path:"stream_id" example:"main-archive" doc:"Stream identifier"`
	},
	) (*models.StreamSlimResponse, error) {
		return &models.StreamSlimResponse{Body: models.StreamSlimData{StreamID: input.StreamID}}, nil
	})
}
