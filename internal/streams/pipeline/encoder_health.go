package pipeline

import (
	"time"

	"github.com/smazurov/videonode/internal/metrics"
	"github.com/smazurov/videonode/internal/process"
)

// EncoderHealth reads process identity and current-run progress without starting an encoder.
func (p *Pipeline) EncoderHealth(streamID string, now time.Time) metrics.EncoderHealth {
	info := p.pool.GetStatus(EncoderIDFor(streamID))
	empty := metrics.Activity{}.Snapshot(now)
	out := metrics.EncoderHealth{
		State:  string(info.State),
		FFmpeg: metrics.FFmpegHealth{Progress: empty, Frames: empty},
	}
	if !info.StartedAt.IsZero() {
		out.StartedAtUS = info.StartedAt.UnixMicro()
	}
	if info.State != process.StateRunning && info.State != process.StateStarting {
		return out
	}
	p.mu.Lock()
	c := p.collectors[streamID]
	p.mu.Unlock()
	if c != nil {
		out.FFmpeg = c.Health(out.StartedAtUS, now)
	}
	return out
}
