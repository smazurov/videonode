package api

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/smazurov/videonode/internal/events"
	"github.com/smazurov/videonode/internal/hostmetrics"
	"github.com/smazurov/videonode/internal/streams/pipeline"
)

// ProcessesProvider is what /api/processes needs from the daemon — a
// Snapshot() that returns the current view of supervised stages and a
// RestartProcess() that bounces one stage by its pool id. Pipeline
// implements both directly. Defined here (the consumer side) to keep the
// interface tiny and inversion-of-deps friendly.
type ProcessesProvider interface {
	Snapshot() []pipeline.ProcessView
	// RestartProcess bounces the supervised stage named by its internal
	// pool id ("producer:<id>" / "composer:<id>" / "encoder:<stream-id>").
	// Returns pipeline.ErrNoSuchProcess when the id isn't supervised.
	RestartProcess(id string) error
}

// ProcessEntry is the API-facing per-stage row. Mirrors pipeline.ProcessView
// but normalizes the kind discriminator to the user-facing entity names
// ("source" / "composer" / "encoder") — the legacy "producer" string is
// rewritten to "source" here so the API surface matches the canonical
// [[sources]] / [[composers]] / [[streams]] config shape.
type ProcessEntry struct {
	ID           string   `json:"id" doc:"Pool key (e.g. 'source:hdmi0' / 'composer:cam-front'); 'self' for the daemon row"`
	Kind         string   `json:"kind" enum:"source,composer,encoder,daemon" doc:"Entity kind for this stage ('daemon' = the videonode process itself)"`
	StreamID     string   `json:"stream_id,omitempty" doc:"User-facing stream id (empty for shared sources)"`
	State        string   `json:"state" doc:"Pool state: idle/starting/running/stopping/error"`
	PID          int      `json:"pid,omitempty" doc:"OS pid when running; 0 otherwise"`
	StartedAtUS  int64    `json:"started_at_us,omitempty" doc:"Unix microseconds at Start; 0 when never started"`
	RestartCount int      `json:"restart_count,omitempty" doc:"Times the supervisor restarted this stage"`
	LastError    string   `json:"last_error,omitempty" doc:"Most recent error from the supervisor"`
	RSSBytes     int64    `json:"rss_bytes,omitempty" doc:"Resident set size in bytes"`
	CPUPercent   float64  `json:"cpu_percent,omitempty" doc:"CPU usage as percentage (0-100 per core)"`
	Device       string   `json:"device,omitempty" doc:"Device id (sources only)"`
	Refcount     int      `json:"refcount,omitempty" doc:"Number of streams holding this source (sources only)"`
	Consumers    []string `json:"consumers,omitempty" doc:"Stream ids holding this source (sources only; sorted)"`

	// Device-global hardware utilization — daemon ('self') row only.
	RKMPP []hostmetrics.RKMPPCore  `json:"rkmpp,omitempty" doc:"Per-core Rockchip MPP codec load (host row only)"`
	GPU   *hostmetrics.DevfreqLoad `json:"gpu,omitempty" doc:"Mali GPU devfreq load (host row only)"`
	NPU   *hostmetrics.DevfreqLoad `json:"npu,omitempty" doc:"RKNN NPU devfreq load (host row only)"`
}

// ProcessesListResponse is the response body for GET /api/processes.
type ProcessesListResponse struct {
	Body struct {
		Processes []ProcessEntry `json:"processes" doc:"All supervised pipeline stages"`
	}
}

// RegisterProcessesRoutes registers the /api/processes endpoint on the
// given huma.API.
func RegisterProcessesRoutes(api huma.API, provider ProcessesProvider) {
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

	huma.Register(api, huma.Operation{
		OperationID: "restart-process",
		Method:      http.MethodPost,
		Path:        "/api/processes/{id}/restart",
		Summary:     "Restart a supervised pipeline process",
		Description: "Bounce one supervised stage (source / composer / encoder) through the " +
			"process pool. The id is the pool key as returned by GET /api/processes " +
			"('source:<id>' / 'composer:<id>' / 'encoder:<stream-id>'). Sources and " +
			"composers are re-applied (the gRPC control plane is re-established); a running " +
			"encoder is bounced while an idle one (no reader attached) is left down.",
		Tags:     []string{"processes"},
		Errors:   []int{401, 404, 500},
		Security: withAuth(),
	}, func(_ context.Context, input *struct {
		ID string `path:"id" pattern:"^(source|composer|encoder):[a-zA-Z0-9_-]+$" example:"source:hdmi0" doc:"Process pool id (kind:entity-id) as returned by GET /api/processes"`
	},
	) (*struct{}, error) {
		if err := provider.RestartProcess(denormalizeProcessID(input.ID)); err != nil {
			if errors.Is(err, pipeline.ErrNoSuchProcess) {
				return nil, huma.Error404NotFound("no such supervised process: "+input.ID, err)
			}
			return nil, huma.Error500InternalServerError("failed to restart process", err)
		}
		return &struct{}{}, nil
	})
}

// toProcessEntries projects pipeline.ProcessView rows onto the API shape,
// rewriting the legacy "producer" kind label to "source".
func toProcessEntries(views []pipeline.ProcessView) []ProcessEntry {
	out := make([]ProcessEntry, len(views))
	for i, v := range views {
		out[i] = ProcessEntry{
			ID:           normalizeProcessID(v.ID),
			Kind:         normalizeKind(v.Kind),
			StreamID:     v.StreamID,
			State:        v.State,
			PID:          v.PID,
			StartedAtUS:  v.StartedAtUS,
			RestartCount: v.RestartCount,
			LastError:    v.LastError,
			RSSBytes:     v.RSSBytes,
			CPUPercent:   v.CPUPercent,
			RKMPP:        v.RKMPP,
			GPU:          v.GPU,
			NPU:          v.NPU,
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

// normalizeProcessID rewrites the internal "producer:" pool-key prefix to
// the API-facing "source:" so /api/processes ids match the canonical
// source/composer/encoder vocabulary and round-trip with restart.
func normalizeProcessID(id string) string {
	if rest, ok := strings.CutPrefix(id, "producer:"); ok {
		return "source:" + rest
	}
	return id
}

// normalizeProcessesEvent rewrites each row of a ProcessesEvent from the
// pipeline's internal pool vocabulary to the user-facing one — the same
// "producer"→"source" edge translation applied to the REST /api/processes
// rows — so the SSE push and the REST poll key on identical ids and kinds.
func normalizeProcessesEvent(e events.ProcessesEvent) events.ProcessesEvent {
	out := e
	out.Processes = make([]events.ProcessInfo, len(e.Processes))
	for i, p := range e.Processes {
		p.ID = normalizeProcessID(p.ID)
		p.Kind = normalizeKind(p.Kind)
		out.Processes[i] = p
	}
	return out
}

// denormalizeProcessID is the inverse of normalizeProcessID: it maps an
// API-facing "source:" id back to the internal "producer:" pool key
// before the request reaches the process pool.
func denormalizeProcessID(id string) string {
	if rest, ok := strings.CutPrefix(id, "source:"); ok {
		return "producer:" + rest
	}
	return id
}
