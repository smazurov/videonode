package process

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/smazurov/videonode/internal/logging"
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

	prevTicks int64
	prevWall  time.Time
	cpuPct    float64
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

	return &pool{
		opts:      *opts,
		processes: make(map[string]*managedProcess),
		logger:    logger,
		ctx:       ctx,
		cancel:    cancel,
	}
}

// Start starts a process by ID.
func (p *pool) Start(id string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if proc, exists := p.processes[id]; exists {
		if proc.state == StateRunning || proc.state == StateStarting {
			return fmt.Errorf("process %s already running", id)
		}
	}

	command, err := p.opts.CommandProvider(id)
	if err != nil {
		return fmt.Errorf("failed to generate command: %w", err)
	}

	return p.startProcess(id, command)
}

// startProcess starts a process with the given command (must hold lock).
func (p *pool) startProcess(id string, command string) error {
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

	p.notifyStateChange(id, StateIdle, StateStarting, nil)

	p.wg.Go(func() {
		defer close(mp.done)
		p.runProcess(ctx, mp)
	})

	return nil
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
		p.logger.Error("Process exited unexpectedly", "id", mp.id, "exit_code", exitCode)
	}
	newState := mp.state
	lastErr := mp.lastError
	p.mu.Unlock()

	p.notifyStateChange(mp.id, oldState, newState, lastErr)
	p.logger.Info("Process stopped", "id", mp.id, "exit_code", exitCode)
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
	p.logger.Info("Stopping process", "id", id)

	mp.cancel()
	mp.proc.Shutdown()

	select {
	case <-mp.done:
	case <-time.After(10 * time.Second):
		p.logger.Warn("Timeout waiting for process to stop", "id", id)
	}

	p.mu.Lock()
	delete(p.processes, id)
	p.mu.Unlock()

	return nil
}

// Restart stops and restarts a process.
func (p *pool) Restart(id string) error {
	p.logger.Info("Restarting process", "id", id)
	if err := p.Stop(id); err != nil {
		return fmt.Errorf("failed to stop process: %w", err)
	}
	return p.Start(id)
}

// GetStatus returns process info.
func (p *pool) GetStatus(id string) *Info {
	p.mu.RLock()
	mp, exists := p.processes[id]
	if !exists {
		p.mu.RUnlock()
		return &Info{ID: id, State: StateIdle}
	}

	pid := 0
	if mp.proc != nil && mp.proc.cmd != nil && mp.proc.cmd.Process != nil {
		pid = mp.proc.cmd.Process.Pid
	}

	info := &Info{
		ID:           id,
		Kind:         mp.kind,
		State:        mp.state,
		PID:          pid,
		StartedAt:    mp.startedAt,
		RestartCount: mp.restartCount,
		LastError:    mp.lastError,
	}
	prevTicks, prevWall := mp.prevTicks, mp.prevWall
	state := mp.state
	p.mu.RUnlock()

	if pid > 0 && state == StateRunning {
		if ps, err := readProcStat(pid); err == nil {
			info.RSSBytes = ps.RSSBytes
			now := time.Now()
			ticks := ps.UtimeTicks + ps.StimeTicks
			var cpuPct float64
			if !prevWall.IsZero() {
				dt := now.Sub(prevWall).Seconds()
				if dt > 0 {
					cpuPct = float64(ticks-prevTicks) / userHZ / dt * 100
				}
			}
			info.CPUPercent = cpuPct

			p.mu.Lock()
			mp.prevTicks = ticks
			mp.prevWall = now
			mp.cpuPct = cpuPct
			p.mu.Unlock()
		}
	}

	return info
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
