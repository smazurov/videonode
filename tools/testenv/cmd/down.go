package cmd

import (
	"database/sql"
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

// DownCmd tears down an env. With no positional arg, the current
// session's env (resolved from $CLAUDE_SESSION_ID) is targeted.
type DownCmd struct {
	EnvID string `arg:"" optional:"" help:"Env id to tear down. Defaults to the current session's env."`
}

func (c *DownCmd) Run(ctx *Context) error {
	s, err := ctx.OpenStore()
	if err != nil {
		return err
	}
	defer s.Close()

	ReapBefore(s)

	envID := c.EnvID
	if envID == "" {
		session := ctx.SessionID
		if session == "" {
			return errors.New("no env id given and no CLAUDE_SESSION_ID to resolve current session's env")
		}
		e, err := s.GetEnvBySession(session)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				fmt.Fprintf(stdout(), "no env owned by session %s\n", session)
				return nil
			}
			return err
		}
		envID = e.ID
	}

	e, err := s.GetEnv(envID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			fmt.Fprintf(stdout(), "no such env: %s\n", envID)
			return nil
		}
		return err
	}

	// Best-effort SIGTERM to the daemon process group, then SIGKILL
	// after a brief pause if it's still alive.
	if e.OwnerPID > 0 {
		_ = unix.Kill(-e.OwnerPID, unix.SIGTERM) // negative pid = process group
		_ = unix.Kill(e.OwnerPID, unix.SIGTERM)  // fall back if no pgrp
	}

	// Even if the kill couldn't reach the process, drop the registry
	// row so the slot frees up. Reap will catch the orphan PID next time.
	if err := s.DeleteEnv(envID); err != nil {
		return err
	}

	fmt.Fprintf(stdout(), "env %s torn down (pid %d signaled)\n", envID, e.OwnerPID)
	_ = os.Getenv // appease the linter if it ever complains
	return nil
}
