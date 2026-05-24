package cmd

import (
	"database/sql"
	"errors"
	"fmt"
)

// LeaseCmd acquires a named resource lease for the current session's env.
type LeaseCmd struct {
	ResourceID string `arg:"" help:"Resource id, e.g. device:/dev/video0."`
}

func (c *LeaseCmd) Run(ctx *Context) error {
	s, err := ctx.OpenStore()
	if err != nil {
		return err
	}
	defer s.Close()
	ReapBefore(s)

	if ctx.SessionID == "" {
		return errors.New("no CLAUDE_SESSION_ID set; cannot tie lease to a session")
	}
	env, err := s.GetEnvBySession(ctx.SessionID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errors.New("current session owns no env; call `testenv up` first")
		}
		return err
	}
	if holder, herr := s.LeaseHolder(c.ResourceID); herr == nil && holder != "" {
		return formatLeaseConflict(s, c.ResourceID, holder)
	}
	if err := s.LeaseAcquire(c.ResourceID, env.ID); err != nil {
		return err
	}
	fmt.Fprintf(stdout(), "lease %s acquired by env %s\n", c.ResourceID, env.ID)
	return nil
}

// ReleaseCmd drops a named resource lease.
type ReleaseCmd struct {
	ResourceID string `arg:"" help:"Resource id."`
}

func (c *ReleaseCmd) Run(ctx *Context) error {
	s, err := ctx.OpenStore()
	if err != nil {
		return err
	}
	defer s.Close()
	if err := s.LeaseRelease(c.ResourceID); err != nil {
		return err
	}
	fmt.Fprintf(stdout(), "lease %s released\n", c.ResourceID)
	return nil
}

// ReleaseSessionCmd releases every env (and cascaded leases) owned by
// a session. Called by the SessionEnd hook.
type ReleaseSessionCmd struct {
	SessionID string `arg:"" optional:"" help:"Session id (default: $CLAUDE_SESSION_ID)."`
}

func (c *ReleaseSessionCmd) Run(ctx *Context) error {
	s, err := ctx.OpenStore()
	if err != nil {
		return err
	}
	defer s.Close()

	session := c.SessionID
	if session == "" {
		session = ctx.SessionID
	}
	if session == "" {
		return errors.New("no session id given and no CLAUDE_SESSION_ID set")
	}

	// Best-effort: signal the daemon(s) before dropping rows, so we
	// don't leave orphan processes that the next reap then has to
	// catch. DownCmd's signal logic is duplicated here to avoid an
	// import cycle; both call sites are tiny.
	envs, _ := s.ListEnvs()
	for _, e := range envs {
		if e.OwnerSession != session {
			continue
		}
		signalDaemon(e.OwnerPID)
	}

	released, err := s.DeleteEnvsForSession(session)
	if err != nil {
		return err
	}
	if len(released) == 0 {
		fmt.Fprintf(stdout(), "no envs owned by session %s\n", session)
		return nil
	}
	fmt.Fprintf(stdout(), "released %d env(s): %v\n", len(released), released)
	return nil
}
