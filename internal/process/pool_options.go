package process

import "github.com/smazurov/videonode/internal/logging"

// CommandProvider generates the command string for a process ID.
// This allows domain-specific command generation (e.g., FFmpeg commands from stream config).
type CommandProvider func(id string) (command string, err error)

// StateChangeCallback is called when a process state changes.
// Used for domain-specific reactions (e.g., events, LED control).
type StateChangeCallback func(id string, oldState, newState State, err error)

// StatsChangeCallback is called once per stats sample after the pool has
// refreshed every running process's CPU%/RSS. It is a notification, not a
// payload: the consumer re-reads GetStatus for the processes it cares about.
// Fired when at least one process was sampled; with SelfSampler set it also
// fires on every tick so the daemon footprint keeps streaming while idle.
type StatsChangeCallback func()

// RemovedCallback is called after a process has been removed from the pool
// (its final transition already fired via OnStateChange). Lets consumers emit
// an explicit "gone" signal — a removed process produces no further state or
// stats events, so without this the last-known row would linger.
type RemovedCallback func(id string)

// Configurer configures a Process before it starts.
// Used for domain-specific setup (e.g., log parser, output handler).
type Configurer func(id string, proc *Process)

// PoolOptions configures a new Pool.
type PoolOptions struct {
	// CommandProvider generates the command for a given process ID (required).
	CommandProvider CommandProvider

	// OnStateChange is called when process state transitions (optional).
	OnStateChange StateChangeCallback

	// OnStats is called after each stats sample refreshes CPU%/RSS for the
	// running processes (optional). Used to push process metrics over SSE
	// without a steady-state REST poll.
	OnStats StatsChangeCallback

	// OnRemove is called after a process is removed from the pool (optional).
	// Used to push an explicit removal so subscribers drop the stale row.
	OnRemove RemovedCallback

	// SelfSampler, when non-nil, makes the pool sample the daemon's own
	// CPU%/RSS on every stats tick and surface it via Self(). With self
	// monitoring enabled the stats tick always fires OnStats — even with
	// zero supervised children — so the daemon footprint keeps refreshing
	// while the pipeline is idle.
	SelfSampler *SelfSampler

	// SelfStartedAtUS is the daemon start time (Unix microseconds) reported
	// as the self row's StartedAt. Only meaningful when SelfSampler is set.
	SelfStartedAtUS int64

	// ConfigureProcess allows customization of the Process before start (optional).
	ConfigureProcess Configurer

	// Logger for pool operations. If nil, uses slog.Default().
	Logger logging.Logger
}
