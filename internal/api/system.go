package api

// /api/system — daemon-wide resource summary for the operator status
// bar: how long the daemon has been up, and the combined CPU/memory
// footprint of the whole pipeline (the daemon itself plus every
// supervised stage). The per-stage breakdown lives at /api/processes;
// this endpoint is the single rolled-up number the UI shows at a glance.

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
)

// SystemStatsError names a supervised stage that is in the error state,
// with its most recent error message (when one was captured).
type SystemStatsError struct {
	ID      string `json:"id" doc:"Pool key of the erroring stage"`
	Message string `json:"message,omitempty" doc:"Most recent error from the supervisor"`
}

// SystemStatsResponse is the response body for GET /api/system.
type SystemStatsResponse struct {
	Body struct {
		StartedAtUS  int64              `json:"started_at_us" doc:"Daemon start time in Unix microseconds; render as uptime"`
		CPUPercent   float64            `json:"cpu_percent" doc:"Combined CPU usage (0-100 per core) of the daemon and all supervised stages"`
		RSSBytes     int64              `json:"rss_bytes" doc:"Combined resident set size in bytes of the daemon and all supervised stages"`
		ProcessCount int                `json:"process_count" doc:"Number of processes counted (the daemon plus running supervised stages)"`
		ErrorCount   int                `json:"error_count" doc:"Number of supervised stages currently in the error state"`
		Errors       []SystemStatsError `json:"errors,omitempty" doc:"Supervised stages currently in the error state (sorted by id)"`
	}
}

// registerSystemRoutes wires GET /api/system. The handler samples the
// daemon's own CPU/RSS via the Server's SelfSampler and folds in the
// running supervised stages from the ProcessesProvider snapshot.
func (s *Server) registerSystemRoutes() {
	huma.Register(s.api, huma.Operation{
		OperationID: "system-stats",
		Method:      http.MethodGet,
		Path:        "/api/system",
		Summary:     "Daemon-wide resource summary",
		Description: "Returns the daemon uptime and the combined CPU/memory footprint of the " +
			"daemon plus every running supervised stage (sources, composers, encoders). " +
			"The per-stage breakdown is available at /api/processes.",
		Tags:     []string{"system"},
		Errors:   []int{401},
		Security: withAuth(),
	}, func(_ context.Context, _ *struct{}) (*SystemStatsResponse, error) {
		resp := &SystemStatsResponse{}
		resp.Body.StartedAtUS = s.startedAtUS

		rss, cpu := s.selfSampler.Sample()
		count := 1 // the daemon itself

		if s.options != nil && s.options.ProcessesProvider != nil {
			for _, v := range s.options.ProcessesProvider.Snapshot() {
				if v.State == "error" {
					resp.Body.Errors = append(resp.Body.Errors, SystemStatsError{
						ID:      v.ID,
						Message: v.LastError,
					})
				}
				// Only running stages count toward the live footprint; a
				// dead/errored stage keeps its last pid but consumes nothing.
				if v.State != "running" || v.PID <= 0 {
					continue
				}
				rss += v.RSSBytes
				cpu += v.CPUPercent
				count++
			}
		}

		resp.Body.RSSBytes = rss
		resp.Body.CPUPercent = cpu
		resp.Body.ProcessCount = count
		resp.Body.ErrorCount = len(resp.Body.Errors)
		return resp, nil
	})
}
