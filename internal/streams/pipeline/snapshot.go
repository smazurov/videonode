package pipeline

import (
	"maps"
	"sort"
	"strings"

	"github.com/smazurov/videonode/internal/hostmetrics"
)

// ProcessView is the per-stage snapshot the /api/processes endpoint
// surfaces. Built from the pool + stages map + source registry. Each
// row carries pool state, kind discriminator, and ids so the operator
// dashboard can answer ownership questions without further joins.
type ProcessView struct {
	ID           string  `json:"id" doc:"Pool key (e.g. 'producer:hdmi0' / 'composer:main' / 'encoder:cam')"`
	Kind         string  `json:"kind" doc:"'producer' | 'composer' | 'encoder'"`
	StreamID     string  `json:"stream_id" doc:"User-facing stream id (empty for sources/composers)"`
	SourceID     string  `json:"source_id,omitempty" doc:"Source id (producers only)"`
	State        string  `json:"state" doc:"Pool state: idle/starting/running/stopping/error"`
	PID          int     `json:"pid,omitempty" doc:"OS pid when running; 0 otherwise"`
	StartedAtUS  int64   `json:"started_at_us,omitempty" doc:"Unix microseconds at Start; 0 when never started"`
	RestartCount int     `json:"restart_count,omitempty" doc:"Times the supervisor restarted this stage"`
	LastError    string  `json:"last_error,omitempty" doc:"Most recent error from the supervisor"`
	RSSBytes     int64   `json:"rss_bytes,omitempty" doc:"Resident set size in bytes"`
	CPUPercent   float64 `json:"cpu_percent,omitempty" doc:"CPU usage as percentage (0-100 per core)"`

	// Device-global hardware utilization — populated on the 'self' (daemon)
	// row only, when the host exposes the hardware. The kernel reports these
	// per-IP-block, not per-process, so they do not attach to supervised stages.
	RKMPP []hostmetrics.RKMPPCore  `json:"rkmpp,omitempty" doc:"Per-core Rockchip MPP codec load (host row only)"`
	GPU   *hostmetrics.DevfreqLoad `json:"gpu,omitempty" doc:"Mali GPU devfreq load (host row only)"`
	NPU   *hostmetrics.DevfreqLoad `json:"npu,omitempty" doc:"RKNN NPU devfreq load (host row only)"`
}

// Snapshot returns the current set of supervised processes joined with
// stage metadata. Sorted by ID for deterministic output.
func (p *Pipeline) Snapshot() []ProcessView {
	ids := p.pool.IDs()
	p.mu.Lock()
	stagesCopy := make(map[string]Stage, len(p.stages))
	maps.Copy(stagesCopy, p.stages)
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
			RSSBytes:     info.RSSBytes,
			CPUPercent:   info.CPUPercent,
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
			view.SourceID = strings.TrimPrefix(id, "producer:")
		}
		out = append(out, view)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })

	// The daemon's own row (when self monitoring is enabled) is pinned ahead
	// of the supervised stages so the operator dashboard reads top-down from
	// host → pipeline. It is the single input the InfoBar rollup needs beyond
	// the supervised stages.
	if self := p.pool.Self(); self != nil {
		view := ProcessView{
			ID:         self.ID,
			Kind:       self.Kind,
			State:      string(self.State),
			PID:        self.PID,
			RSSBytes:   self.RSSBytes,
			CPUPercent: self.CPUPercent,
		}
		if !self.StartedAt.IsZero() {
			view.StartedAtUS = self.StartedAt.UnixMicro()
		}
		host := hostmetrics.Sample()
		view.RKMPP = host.RKMPP
		view.GPU = host.GPU
		view.NPU = host.NPU
		out = append([]ProcessView{view}, out...)
	}
	return out
}
