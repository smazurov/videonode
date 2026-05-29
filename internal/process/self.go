package process

import (
	"os"
	"sync"
	"time"
)

// SelfSampler tracks this process's own CPU/RSS over time, computing CPU%
// as a delta between successive samples — the same method the pool uses
// for its supervised children. Zero value is ready to use; Sample is
// safe for concurrent callers.
type SelfSampler struct {
	mu        sync.Mutex
	prevTicks int64
	prevWall  time.Time
}

// Sample reads /proc/self/stat and returns the current resident set size
// in bytes and CPU percent (0-100 per core) since the previous call.
// CPU is 0 on the first call (no prior sample to diff against) and on any
// procfs read error.
func (s *SelfSampler) Sample() (rssBytes int64, cpuPct float64) {
	ps, err := readProcStat(os.Getpid())
	if err != nil {
		return 0, 0
	}
	ticks := ps.UtimeTicks + ps.StimeTicks
	now := time.Now()

	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.prevWall.IsZero() {
		if dt := now.Sub(s.prevWall).Seconds(); dt > 0 {
			cpuPct = float64(ticks-s.prevTicks) / userHZ / dt * 100
		}
	}
	s.prevTicks = ticks
	s.prevWall = now
	return ps.RSSBytes, cpuPct
}
