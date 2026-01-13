package process

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
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

	// StopAll gracefully stops all running processes.
	StopAll()
}

// managedProcess tracks a running process within the pool.
type managedProcess struct {
	proc         *Process
	id           string
	state        State
	startedAt    time.Time
	restartCount int
	lastError    error
	cancel       context.CancelFunc
	done         chan struct{}
}

// pool implements the Pool interface.
type pool struct {
	opts      PoolOptions
	processes map[string]*managedProcess
	mu        sync.RWMutex
	logger    *slog.Logger
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

	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		defer close(mp.done)
		p.runProcess(ctx, mp)
	}()

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
		mp.state = StateIdle
	case exitCode != 0:
		mp.state = StateError
		mp.lastError = fmt.Errorf("process exited with code %d", exitCode)
		p.logger.Error("Process crashed", "id", mp.id, "exit_code", exitCode)
	default:
		mp.state = StateIdle
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
	defer p.mu.RUnlock()

	mp, exists := p.processes[id]
	if !exists {
		return &Info{ID: id, State: StateIdle}
	}

	return &Info{
		ID:           id,
		State:        mp.state,
		StartedAt:    mp.startedAt,
		RestartCount: mp.restartCount,
		LastError:    mp.lastError,
	}
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

	for _, id := range ids {
		_ = p.Stop(id)
	}

	p.wg.Wait()
	p.logger.Info("All processes stopped")
}

// notifyStateChange invokes the OnStateChange callback if configured.
func (p *pool) notifyStateChange(id string, oldState, newState State, err error) {
	if p.opts.OnStateChange != nil {
		p.opts.OnStateChange(id, oldState, newState, err)
	}
}
