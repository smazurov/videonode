package pipeline

import (
	"sort"
	"strings"
)

// ProcessView is the per-stage snapshot the /api/processes endpoint
// surfaces. Built by Pipeline.Snapshot from the pool + stage map +
// producer registry. Fields chosen to make the operator dashboard
// answerable without further joins: "which stages exist? for which
// streams? what's their PID, state, refcount, error?"
type ProcessView struct {
	ID           string   `json:"id" doc:"Pool key (e.g. 'producer:hdmi0' / 'composer:cam-front')"`
	Kind         string   `json:"kind" doc:"'producer' | 'composer' | 'encoder'"`
	StreamID     string   `json:"stream_id" doc:"User-facing stream id (empty for shared producers)"`
	State        string   `json:"state" doc:"Pool state: idle/starting/running/stopping/error"`
	PID          int      `json:"pid,omitempty" doc:"OS pid when running; 0 otherwise"`
	StartedAtUS  int64    `json:"started_at_us,omitempty" doc:"Unix microseconds at Start; 0 when never started"`
	RestartCount int      `json:"restart_count,omitempty" doc:"Times the supervisor restarted this stage"`
	LastError    string   `json:"last_error,omitempty" doc:"Most recent error from the supervisor"`
	Device       string   `json:"device,omitempty" doc:"Device id (producers only)"`
	Refcount     int      `json:"refcount,omitempty" doc:"Number of streams holding this producer (producers only)"`
	Consumers    []string `json:"consumers,omitempty" doc:"Stream ids holding this producer (producers only; sorted)"`
}

// Snapshot returns the current set of supervised processes joined with
// stage metadata + producer-registry refcounts. Sorted by ID for
// deterministic output (UI rendering, snapshot diff).
func (p *Pipeline) Snapshot() []ProcessView {
	ids := p.pool.IDs()
	p.mu.Lock()
	stagesCopy := make(map[string]Stage, len(p.stages))
	for k, v := range p.stages {
		stagesCopy[k] = v
	}
	p.mu.Unlock()

	out := make([]ProcessView, 0, len(ids))
	for _, id := range ids {
		info := p.pool.GetStatus(id)
		view := ProcessView{
			ID:           id,
			State:        string(info.State),
			Kind:         info.Kind,
			PID:          info.PID,
			RestartCount: info.RestartCount,
		}
		if !info.StartedAt.IsZero() {
			view.StartedAtUS = info.StartedAt.UnixMicro()
		}
		if info.LastError != nil {
			view.LastError = info.LastError.Error()
		}
		if stage, ok := stagesCopy[id]; ok {
			view.StreamID = stage.StreamID()
			if view.Kind == "" {
				view.Kind = stage.Kind().String()
			}
		}
		if view.Kind == KindProducer.String() {
			device := strings.TrimPrefix(id, "producer:")
			view.Device = device
			view.Refcount = p.producers.Refcount(device)
			view.Consumers = p.producers.ConsumersOf(device)
		}
		out = append(out, view)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
