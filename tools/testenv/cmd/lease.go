package cmd

import (
	"fmt"

	"github.com/smazurov/videonode/tools/testenv/internal/envctl"
)

type LeaseCmd struct {
	ResourceID string `arg:"" help:"Resource id, e.g. device:/dev/video0."`
}

func (c *LeaseCmd) Run(ctx *Context) error {
	if err := envctl.Lease(ctx.Ctx, envctl.LeaseParams{
		StatePath:  ctx.StatePath,
		Session:    ctx.SessionID,
		ResourceID: c.ResourceID,
	}); err != nil {
		return err
	}
	fmt.Fprintf(stdout(), "lease %s acquired\n", c.ResourceID)
	return nil
}

type ReleaseCmd struct {
	ResourceID string `arg:"" help:"Resource id."`
}

func (c *ReleaseCmd) Run(ctx *Context) error {
	if err := envctl.Release(ctx.Ctx, ctx.StatePath, c.ResourceID); err != nil {
		return err
	}
	fmt.Fprintf(stdout(), "lease %s released\n", c.ResourceID)
	return nil
}

type ReleaseSessionCmd struct {
	SessionID string `arg:"" optional:"" help:"Session id (default: $CLAUDE_SESSION_ID)."`
}

func (c *ReleaseSessionCmd) Run(ctx *Context) error {
	session := c.SessionID
	if session == "" {
		session = ctx.SessionID
	}
	released, err := envctl.ReleaseSession(ctx.Ctx, ctx.StatePath, session)
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
