package cmd

import (
	"fmt"

	"github.com/smazurov/videonode/tools/testenv/internal/envctl"
)

// ReapCmd sweeps and removes leases for dead sessions.
type ReapCmd struct{}

// Run executes the reap command.
func (c *ReapCmd) Run(ctx *Context) error {
	r, err := envctl.Reap(ctx.Ctx, ctx.StatePath)
	if err != nil {
		return err
	}
	if len(r.Released) == 0 {
		_, _ = fmt.Fprintln(stdout(), "no stale envs to reap")
		return nil
	}
	_, _ = fmt.Fprintf(stdout(), "reaped %d env(s): %v\n", len(r.Released), r.Released)
	return nil
}
