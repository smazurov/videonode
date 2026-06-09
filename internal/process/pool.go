package process

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/smazurov/videonode/internal/logging"
)

// Pool key and kind for the synthetic daemon row surfaced via Self() when
// self monitoring is enabled. The daemon is not a supervised child — it has
// no command and is never started/stopped/restarted — so it lives outside the
// processes map and carries an id with no "kind:entity" colon.
const (
	SelfID   = "self"
	SelfKind = "daemon"
)

// Pool manages multiple named processes with lifecycle control.
type Pool interface {
	// Start starts a process by ID. Returns error if already running.
	Start(id string) error

	// Stop gracefully stops a process by ID.
	Stop(id string) error

	// Restart stops and restarts a process.
	Restart(id string) error

	// GetStatus returns process info. Returns idle state if not found.
	GetStatus(id string) *Info

	// IsRunning checks if a process is currently running.
	IsRunning(id string) bool

	// SetKind tags a managed process with a free-form classifier
	// (returned via Info.Kind). Used by the Pipeline to expose stage
	// kind ("producer"/"composer"/"encoder") to operator UIs without
	// id-string parsing. No-op for unknown ids.
	SetKind(id, kind string)

	// IDs returns a snapshot of currently-tracked process ids.
	IDs() []string

	// Self returns the daemon's own process row (CPU%/RSS/uptime) when self
	// monitoring is enabled via PoolOptions.SelfSampler, or nil otherwise.
	Self() *Info

	// StopAll gracefully stops all running processes.
	StopAll()
}

// managedProcess tracks a running process within the pool.
type managedProcess struct {
	proc         *Process
	id           string
	kind         string
	state        State
	startedAt    time.Time
	restartCount int
	lastError    error
	cancel       context.CancelFunc
	done         chan struct{}

	// Stats populated by the background poller — never written by GetStatus.
	rssBytes  int64
	cpuPct    float64
	prevTicks int64
	prevWall  time.Time
}

// pool implements the Pool interface.
type pool struct {
	opts      PoolOptions
	processes map[string]*managedProcess
	mu        sync.RWMutex
	logger    logging.Logger
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup

	// Cached daemon footprint, refreshed by the stats poller when self
	// monitoring is enabled. Guarded by mu (read by Self()).
	selfRSS int64
	selfCPU float64
}

// NewPool creates a new process pool.
func NewPool(opts *PoolOptions) Pool {
	if opts == nil || opts.CommandProvider == nil {
		panic("PoolOptions with CommandProvider is required")
	}

	ctx, cancel := context.WithCancel(context.Background())

	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}

	p := &pool{
		opts:      *opts,
		processes: make(map[string]*managedProcess),
		logger:    logger,
		ctx:       ctx,
		cancel:    cancel,
	}

	p.wg.Go(func() { p.pollStats(ctx) })

	return p
}

// Start starts a process by ID.
func (p *pool) Start(id string) error {
	p.mu.Lock()

	if proc, exists := p.processes[id]; exists {
		if proc.state == StateRunning || proc.state == StateStarting {
			p.mu.Unlock()
			return fmt.Errorf("process %s already running", id)
		}
	}

	command, err := p.opts.CommandProvider(id)
	if err != nil {
		p.mu.Unlock()
		return fmt.Errorf("failed to generate command: %w", err)
	}

	p.startProcess(id, command)
	p.mu.Unlock()

	p.notifyStateChange(id, StateIdle, StateStarting, nil)
	return nil
}

// startProcess inserts a new managed process and spawns its run
// goroutine. Caller must hold p.mu and must call notifyStateChange
// after releasing the lock.
func (p *pool) startProcess(id string, command string) {
	ctx, cancel := context.WithCancel(p.ctx)

	mp := &managedProcess{
		id:        id,
		state:     StateStarting,
		startedAt: time.Now(),
		cancel:    cancel,
		done:      make(chan struct{}),
	}

	mp.proc = NewProcess(id, command, p.logger)

	if p.opts.ConfigureProcess != nil {
		p.opts.ConfigureProcess(id, mp.proc)
	}

	p.processes[id] = mp

	p.wg.Go(func() {
		defer close(mp.done)
		p.runProcess(ctx, mp)
	})
}

// runProcess runs the process and handles state transitions.
func (p *pool) runProcess(ctx context.Context, mp *managedProcess) {
	p.mu.Lock()
	oldState := mp.state
	mp.state = StateRunning
	p.mu.Unlock()
	p.notifyStateChange(mp.id, oldState, StateRunning, nil)

	exitCode := mp.proc.Run()

	p.mu.Lock()
	oldState = mp.state
	switch {
	case ctx.Err() != nil:
		// Expected exit - we stopped it via Stop()
		mp.state = StateIdle
	default:
		// Process exited on its own - always unexpected
		mp.state = StateError
		mp.lastError = fmt.Errorf("process exited with code %d", exitCode)
		p.logger.Error("Process exited unexpectedly", logging.KeyPoolID, mp.id, logging.KeyExitCode, exitCode)
	}
	newState := mp.state
	lastErr := mp.lastError
	p.mu.Unlock()

	p.notifyStateChange(mp.id, oldState, newState, lastErr)
	p.logger.Info("Process stopped", logging.KeyPoolID, mp.id, logging.KeyExitCode, exitCode)
}

// Stop gracefully stops a process by ID.
func (p *pool) Stop(id string) error {
	p.mu.Lock()
	mp, exists := p.processes[id]
	if !exists {
		p.mu.Unlock()
		return nil
	}

	if mp.state != StateRunning && mp.state != StateStarting {
		p.mu.Unlock()
		return nil
	}

	oldState := mp.state
	mp.state = StateStopping
	p.mu.Unlock()

	p.notifyStateChange(id, oldState, StateStopping, nil)
	p.logger.Info("Stopping process", logging.KeyPoolID, id)

	mp.cancel()
	mp.proc.Shutdown()

	select {
	case <-mp.done:
	case <-time.After(10 * time.Second):
		p.logger.Warn("Timeout waiting for process to stop", logging.KeyPoolID, id)
	}

	p.mu.Lock()
	delete(p.processes, id)
	p.mu.Unlock()

	p.notifyRemoved(id)
	return nil
}

// Restart stops and restarts a process.
func (p *pool) Restart(id string) error {
	p.logger.Info("Restarting process", logging.KeyPoolID, id)
	if err := p.Stop(id); err != nil {
		return fmt.Errorf("failed to stop process: %w", err)
	}
	return p.Start(id)
}

// GetStatus returns process info. Pure cache read — never does I/O or
// takes a write lock. RSS/CPU values are populated by the background
// stats poller.
func (p *pool) GetStatus(id string) *Info {
	p.mu.RLock()
	mp, exists := p.processes[id]
	if !exists {
		p.mu.RUnlock()
		return &Info{ID: id, State: StateIdle}
	}

	info := &Info{
		ID:           id,
		Kind:         mp.kind,
		State:        mp.state,
		PID:          mp.proc.PID(),
		StartedAt:    mp.startedAt,
		RestartCount: mp.restartCount,
		LastError:    mp.lastError,
		RSSBytes:     mp.rssBytes,
		CPUPercent:   mp.cpuPct,
	}
	p.mu.RUnlock()

	return info
}

// statsPollInterval is how often the background poller samples CPU%/RSS for
// every running process. A var (not const) so tests can shrink it.
var statsPollInterval = 2 * time.Second

func (p *pool) pollStats(ctx context.Context) {
	ticker := time.NewTicker(statsPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.refreshStats()
		}
	}
}

func (p *pool) refreshStats() {
	type snap struct {
		id        string
		pid       int
		prevTicks int64
		prevWall  time.Time
	}

	p.mu.RLock()
	snaps := make([]snap, 0, len(p.processes))
	for _, mp := range p.processes {
		if mp.state != StateRunning {
			continue
		}
		pid := mp.proc.PID()
		if pid <= 0 {
			continue
		}
		snaps = append(snaps, snap{
			id: mp.id, pid: pid,
			prevTicks: mp.prevTicks, prevWall: mp.prevWall,
		})
	}
	p.mu.RUnlock()

	type result struct {
		id    string
		rss   int64
		cpu   float64
		ticks int64
		wall  time.Time
	}
	results := make([]result, 0, len(snaps))
	now := time.Now()
	for _, s := range snaps {
		ps, err := readProcStat(s.pid)
		if err != nil {
			continue
		}
		ticks := ps.UtimeTicks + ps.StimeTicks
		var cpuPct float64
		if !s.prevWall.IsZero() {
			dt := now.Sub(s.prevWall).Seconds()
			if dt > 0 {
				cpuPct = float64(ticks-s.prevTicks) / userHZ / dt * 100
			}
		}
		results = append(results, result{
			id: s.id, rss: ps.RSSBytes, cpu: cpuPct,
			ticks: ticks, wall: now,
		})
	}

	if len(results) > 0 {
		p.mu.Lock()
		for _, r := range results {
			mp, ok := p.processes[r.id]
			if !ok {
				continue
			}
			mp.rssBytes = r.rss
			mp.cpuPct = r.cpu
			mp.prevTicks = r.ticks
			mp.prevWall = r.wall
		}
		p.mu.Unlock()
	}

	// Self monitoring keeps the daemon footprint fresh and the stats stream
	// ticking every interval even with zero supervised children, so the
	// operator UI's daemon stats don't freeze while the pipeline is idle.
	if p.opts.SelfSampler != nil {
		rss, cpu := p.opts.SelfSampler.Sample()
		p.mu.Lock()
		p.selfRSS = rss
		p.selfCPU = cpu
		p.mu.Unlock()
	}

	if len(results) == 0 && p.opts.SelfSampler == nil {
		return
	}
	p.notifyStatsChange()
}

// Self returns the daemon's own process row when self monitoring is enabled,
// or nil otherwise. CPU%/RSS come from the cache last refreshed by the stats
// poller; the row is synthetic (no command, never restartable) so it carries
// a bare "self" id and "daemon" kind.
func (p *pool) Self() *Info {
	if p.opts.SelfSampler == nil {
		return nil
	}
	p.mu.RLock()
	rss, cpu := p.selfRSS, p.selfCPU
	p.mu.RUnlock()

	var startedAt time.Time
	if p.opts.SelfStartedAtUS > 0 {
		startedAt = time.UnixMicro(p.opts.SelfStartedAtUS)
	}
	return &Info{
		ID:         SelfID,
		Kind:       SelfKind,
		State:      StateRunning,
		PID:        os.Getpid(),
		StartedAt:  startedAt,
		RSSBytes:   rss,
		CPUPercent: cpu,
	}
}

// SetKind sets the free-form classifier surfaced via Info.Kind for a
// given pool id. Used by Pipeline to tag each managed process with its
// stage kind ("producer" / "composer" / "encoder") so /api/processes
// can group rows without inspecting the id string. No-op for unknown
// ids; safe to call before or after Start.
func (p *pool) SetKind(id, kind string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if mp, ok := p.processes[id]; ok {
		mp.kind = kind
	}
}

// IDs returns a snapshot of currently-tracked process ids (regardless
// of state). Used by Pipeline.Snapshot and the process-manager UI to
// enumerate without holding the pool lock externally.
func (p *pool) IDs() []string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	out := make([]string, 0, len(p.processes))
	for id := range p.processes {
		out = append(out, id)
	}
	return out
}

// IsRunning checks if a process is currently running.
func (p *pool) IsRunning(id string) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()

	mp, exists := p.processes[id]
	return exists && mp.state == StateRunning
}

// StopAll gracefully stops all running processes.
func (p *pool) StopAll() {
	p.logger.Info("Stopping all processes")
	p.cancel()

	p.mu.RLock()
	ids := make([]string, 0, len(p.processes))
	for id := range p.processes {
		ids = append(ids, id)
	}
	p.mu.RUnlock()

	// Fan out — each Stop blocks up to the per-process shutdown timeout, so
	// stopping N processes serially would take N × timeout on a stuck system.
	var stopWg sync.WaitGroup
	stopWg.Add(len(ids))
	for _, id := range ids {
		go func(streamID string) {
			defer stopWg.Done()
			_ = p.Stop(streamID)
		}(id)
	}
	stopWg.Wait()

	p.wg.Wait()
	p.logger.Info("All processes stopped")
}

// notifyStateChange invokes the OnStateChange callback if configured.
func (p *pool) notifyStateChange(id string, oldState, newState State, err error) {
	if p.opts.OnStateChange != nil {
		p.opts.OnStateChange(id, oldState, newState, err)
	}
}

// notifyStatsChange invokes the OnStats callback if configured. Called from
// refreshStats when at least one running process was sampled, or on every tick
// while self monitoring is enabled (so the daemon footprint keeps streaming
// even when no children run).
func (p *pool) notifyStatsChange() {
	if p.opts.OnStats != nil {
		p.opts.OnStats()
	}
}

// notifyRemoved invokes the OnRemove callback if configured. Called from Stop
// after the process has been deleted from the pool.
func (p *pool) notifyRemoved(id string) {
	if p.opts.OnRemove != nil {
		p.opts.OnRemove(id)
	}
}
