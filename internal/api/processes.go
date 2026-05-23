package api

// /api/processes — operator-visible view of the pipeline's supervised
// processes (Producer / Composer / Encoder stages). Joins pool state +
// stage kind + producer refcounts on a single REST surface.
//
// Endpoint surface is read-only; stop/restart actions land in a
// follow-up once authorization semantics are agreed.

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/smazurov/videonode/internal/streams/pipeline"
)

// ProcessesProvider is what /api/processes needs from the daemon — a
// Snapshot() that returns the current view of supervised stages.
// Pipeline implements it directly. Defined here (the consumer side) to
// keep the interface tiny and inversion-of-deps friendly.
type ProcessesProvider interface {
	Snapshot() []pipeline.ProcessView
}

// ProcessesListResponse is the response body for GET /api/processes.
type ProcessesListResponse struct {
	Body struct {
		Processes []pipeline.ProcessView `json:"processes" doc:"All supervised pipeline stages"`
	}
}

// RegisterProcessesRoutes registers the /api/processes endpoint on the
// given huma.API. Wired from server.go when the daemon has a Pipeline
// instance available. No-op when provider is nil (legacy daemon mode).
func RegisterProcessesRoutes(api huma.API, provider ProcessesProvider) {
	if provider == nil {
		return
	}
	huma.Register(api, huma.Operation{
		OperationID: "list-processes",
		Method:      http.MethodGet,
		Path:        "/api/processes",
		Summary:     "List supervised pipeline processes",
		Description: "Returns one row per supervised stage (Producer / Composer / Encoder), " +
			"including pool state, OS pid, restart count, producer refcount, and (for producers) " +
			"the set of streams currently holding each device. Sorted by stage id.",
		Tags:     []string{"processes"},
		Errors:   []int{401},
		Security: withAuth(),
	}, func(_ context.Context, _ *struct{}) (*ProcessesListResponse, error) {
		resp := &ProcessesListResponse{}
		resp.Body.Processes = provider.Snapshot()
		return resp, nil
	})
}
