package process

import "github.com/smazurov/videonode/internal/logging"

// CommandProvider generates the command string for a process ID.
// This allows domain-specific command generation (e.g., FFmpeg commands from stream config).
type CommandProvider func(id string) (command string, err error)

// StateChangeCallback is called when a process state changes.
// Used for domain-specific reactions (e.g., events, LED control).
type StateChangeCallback func(id string, oldState, newState State, err error)

// Configurer configures a Process before it starts.
// Used for domain-specific setup (e.g., log parser, output handler).
type Configurer func(id string, proc *Process)

// PoolOptions configures a new Pool.
type PoolOptions struct {
	// CommandProvider generates the command for a given process ID (required).
	CommandProvider CommandProvider

	// OnStateChange is called when process state transitions (optional).
	OnStateChange StateChangeCallback

	// ConfigureProcess allows customization of the Process before start (optional).
	ConfigureProcess Configurer

	// Logger for pool operations. If nil, uses slog.Default().
	Logger logging.Logger
}
