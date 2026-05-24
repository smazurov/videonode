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

// ProcessEntry is the API-facing per-stage row. Mirrors pipeline.ProcessView
// but normalizes the kind discriminator to the user-facing entity names
// ("source" / "composer" / "encoder") — the legacy "producer" string is
// rewritten to "source" here so the API surface matches the canonical
// [[sources]] / [[composers]] / [[streams]] config shape.
type ProcessEntry struct {
	ID           string   `json:"id" doc:"Pool key (e.g. 'source:hdmi0' / 'composer:cam-front')"`
	Kind         string   `json:"kind" enum:"source,composer,encoder" doc:"Entity kind for this stage"`
	StreamID     string   `json:"stream_id,omitempty" doc:"User-facing stream id (empty for shared sources)"`
	State        string   `json:"state" doc:"Pool state: idle/starting/running/stopping/error"`
	PID          int      `json:"pid,omitempty" doc:"OS pid when running; 0 otherwise"`
	StartedAtUS  int64    `json:"started_at_us,omitempty" doc:"Unix microseconds at Start; 0 when never started"`
	RestartCount int      `json:"restart_count,omitempty" doc:"Times the supervisor restarted this stage"`
	LastError    string   `json:"last_error,omitempty" doc:"Most recent error from the supervisor"`
	Device       string   `json:"device,omitempty" doc:"Device id (sources only)"`
	Refcount     int      `json:"refcount,omitempty" doc:"Number of streams holding this source (sources only)"`
	Consumers    []string `json:"consumers,omitempty" doc:"Stream ids holding this source (sources only; sorted)"`
}

// ProcessesListResponse is the response body for GET /api/processes.
type ProcessesListResponse struct {
	Body struct {
		Processes []ProcessEntry `json:"processes" doc:"All supervised pipeline stages"`
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
		Description: "Returns one row per supervised stage (Source / Composer / Encoder), " +
			"including pool state, OS pid, restart count, source refcount, and (for sources) " +
			"the set of streams currently holding each device. Sorted by stage id.",
		Tags:     []string{"processes"},
		Errors:   []int{401},
		Security: withAuth(),
	}, func(_ context.Context, _ *struct{}) (*ProcessesListResponse, error) {
		resp := &ProcessesListResponse{}
		resp.Body.Processes = toProcessEntries(provider.Snapshot())
		return resp, nil
	})
}

// toProcessEntries projects pipeline.ProcessView rows onto the API shape,
// rewriting the legacy "producer" kind label to "source".
func toProcessEntries(views []pipeline.ProcessView) []ProcessEntry {
	out := make([]ProcessEntry, len(views))
	for i, v := range views {
		out[i] = ProcessEntry{
			ID:           v.ID,
			Kind:         normalizeKind(v.Kind),
			StreamID:     v.StreamID,
			State:        v.State,
			PID:          v.PID,
			StartedAtUS:  v.StartedAtUS,
			RestartCount: v.RestartCount,
			LastError:    v.LastError,
		}
	}
	return out
}

// normalizeKind maps the internal stage-kind string to the API-facing
// entity name. "producer" is the pre-refactor name for the source stage;
// composer/encoder pass through.
func normalizeKind(k string) string {
	if k == "producer" {
		return "source"
	}
	return k
}
