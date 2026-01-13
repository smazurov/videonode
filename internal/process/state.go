package process

import "time"

// State represents the current state of a managed process.
type State string

// Process states.
const (
	StateIdle     State = "idle"     // Not running
	StateStarting State = "starting" // Being started
	StateRunning  State = "running"  // Active
	StateStopping State = "stopping" // Being stopped
	StateError    State = "error"    // Failed to start/crashed
)

// Info contains information about a managed process.
type Info struct {
	ID           string
	State        State
	PID          int
	StartedAt    time.Time
	RestartCount int
	LastError    error
}
