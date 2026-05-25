// Package reaper sweeps the env registry for dead-PID owners.
//
// Reap is cheap (~5ms for a handful of envs), idempotent, and invoked
// on every CLI subcommand entry plus from the SessionStart and
// SessionEnd hooks. It is the primary cleanup mechanism — there is no
// long-running daemon.
package reaper

import (
	"fmt"

	"golang.org/x/sys/unix"

	"github.com/smazurov/videonode/tools/testenv/internal/store"
)

// ReapedEnv holds the identity and data directory of a reaped env.
type ReapedEnv struct {
	ID      string
	DataDir string
}

// Reap removes every env whose owner_pid no longer corresponds to a
// running process.
func Reap(s *store.Store) (released []ReapedEnv, err error) {
	envs, err := s.ListEnvs()
	if err != nil {
		return nil, fmt.Errorf("list envs: %w", err)
	}
	for _, e := range envs {
		if processAlive(e.OwnerPID) {
			continue
		}
		if delErr := s.DeleteEnv(e.ID); delErr == nil {
			released = append(released, ReapedEnv{ID: e.ID, DataDir: e.DataDir})
		}
	}
	return released, nil
}

// processAlive returns true if kill(pid, 0) succeeds — i.e. the
// process exists and we have permission to signal it. EPERM also
// counts as alive (some other user owns it, but it's there).
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := unix.Kill(pid, 0)
	if err == nil {
		return true
	}
	return err == unix.EPERM
}
