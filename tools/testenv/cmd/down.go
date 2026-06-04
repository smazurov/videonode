package cmd

import (
	"fmt"

	"github.com/smazurov/videonode/tools/testenv/internal/envctl"
)

// DownCmd tears down a running test environment.
type DownCmd struct {
	EnvID string `arg:"" optional:"" help:"Env id to tear down. Defaults to the current session's env."`
}

// Run executes the down subcommand.
func (c *DownCmd) Run(ctx *Context) error {
	r, err := envctl.Down(ctx.Ctx, envctl.DownParams{
		StatePath: ctx.StatePath,
		EnvID:     c.EnvID,
		Session:   ctx.SessionID,
	})
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(stdout(), "env %s torn down (pid %d signaled)\n", r.EnvID, r.PID)
	return nil
}
