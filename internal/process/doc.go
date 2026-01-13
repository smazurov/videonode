// Package process provides subprocess lifecycle management.
//
// The package offers two levels of abstraction:
//
// Process wraps os.exec for single subprocess management:
//   - Graceful shutdown with SIGINT and configurable timeout
//   - Force kill with SIGKILL if graceful shutdown times out
//   - Output streaming with pluggable log parsing
//   - Restart support for configuration changes
//
// Pool manages multiple named processes:
//   - Start/Stop/Restart individual processes by ID
//   - State tracking (idle, starting, running, stopping, error)
//   - Callback hooks for command generation and state changes
//   - Configurable process setup via Configurer callback
//   - StopAll for graceful shutdown of all processes
//
// Example usage with Pool:
//
//	pool := process.NewPool(&process.PoolOptions{
//	    CommandProvider: func(id string) (string, error) {
//	        return fmt.Sprintf("ffmpeg -i input_%s.mp4 output_%s.mp4", id, id), nil
//	    },
//	    OnStateChange: func(id string, old, new State, err error) {
//	        log.Printf("Process %s: %s -> %s", id, old, new)
//	    },
//	})
//	pool.Start("stream1")
//	defer pool.StopAll()
package process
