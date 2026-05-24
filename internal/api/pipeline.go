package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
)

// PipelineStateBody is the response body for pipeline state endpoints.
type PipelineStateBody struct {
	Enabled bool `json:"enabled" example:"true" doc:"Whether the pipeline master switch is on"`
}

// PipelineStateResponse wraps PipelineStateBody for API responses.
type PipelineStateResponse struct {
	Body PipelineStateBody
}

// registerPipelineRoutes registers the daemon-wide pipeline master switch endpoints.
func (s *Server) registerPipelineRoutes() {
	huma.Register(s.api, huma.Operation{
		OperationID: "get-pipeline-state",
		Method:      http.MethodGet,
		Path:        "/api/pipeline",
		Summary:     "Get Pipeline State",
		Description: "Return the daemon-wide pipeline master switch state. When false, no stream processes are running.",
		Tags:        []string{"pipeline"},
		Errors:      []int{401},
		Security:    withAuth(),
	}, func(_ context.Context, _ *struct{}) (*PipelineStateResponse, error) {
		return &PipelineStateResponse{
			Body: PipelineStateBody{Enabled: s.streamService.PipelineEnabled()},
		}, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "start-pipeline",
		Method:      http.MethodPost,
		Path:        "/api/pipeline/start",
		Summary:     "Start Pipeline",
		Description: "Flip the daemon-wide pipeline master switch on and start every configured stream.",
		Tags:        []string{"pipeline"},
		Errors:      []int{401, 500},
		Security:    withAuth(),
	}, func(ctx context.Context, _ *struct{}) (*PipelineStateResponse, error) {
		if _, err := s.streamService.StartPipeline(ctx); err != nil {
			return nil, huma.Error500InternalServerError("failed to start pipeline", err)
		}
		return &PipelineStateResponse{
			Body: PipelineStateBody{Enabled: s.streamService.PipelineEnabled()},
		}, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "stop-pipeline",
		Method:      http.MethodPost,
		Path:        "/api/pipeline/stop",
		Summary:     "Stop Pipeline",
		Description: "Flip the daemon-wide pipeline master switch off and stop every supervised process.",
		Tags:        []string{"pipeline"},
		Errors:      []int{401, 500},
		Security:    withAuth(),
	}, func(ctx context.Context, _ *struct{}) (*PipelineStateResponse, error) {
		if _, err := s.streamService.StopPipeline(ctx); err != nil {
			return nil, huma.Error500InternalServerError("failed to stop pipeline", err)
		}
		return &PipelineStateResponse{
			Body: PipelineStateBody{Enabled: s.streamService.PipelineEnabled()},
		}, nil
	})
}
