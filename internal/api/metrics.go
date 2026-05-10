package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/smazurov/videonode/internal/metrics"
)

// MetricsResponse wraps the metrics JSON response.
type MetricsResponse struct {
	Body []metrics.MetricFamily
}

// registerMetricsRoutes registers the JSON metrics endpoint.
func (s *Server) registerMetricsRoutes() {
	huma.Register(s.api, huma.Operation{
		OperationID: "get-metrics",
		Method:      http.MethodGet,
		Path:        "/api/metrics",
		Summary:     "Metrics",
		Description: "Get all Prometheus metrics in JSON format",
		Tags:        []string{"metrics"},
		Security:    withAuth(),
		Errors:      []int{401, 500},
	}, func(_ context.Context, _ *struct{}) (*MetricsResponse, error) {
		data, err := metrics.GetAllMetricsAsJSON()
		if err != nil {
			return nil, huma.Error500InternalServerError("failed to gather metrics", err)
		}
		return &MetricsResponse{Body: data}, nil
	})
}
