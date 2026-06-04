package cmd

import (
	"fmt"

	"github.com/smazurov/videonode/tools/testenv/internal/envctl"
)

// RestartCmd restarts a running test environment and prints its new address.
type RestartCmd struct {
	EnvID string `arg:"" optional:"" help:"Env id to restart. Defaults to the current session's env."`
}

// Run executes the restart subcommand.
func (c *RestartCmd) Run(ctx *Context) error {
	r, err := envctl.Restart(ctx.Ctx, envctl.RestartParams{
		StatePath: ctx.StatePath,
		EnvID:     c.EnvID,
		Session:   ctx.SessionID,
	})
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(stdout(), "env %s restarted\n", r.EnvID)
	_, _ = fmt.Fprintf(stdout(), "  url:  %s\n", r.HTTPURL)
	if r.Auth != "" {
		_, _ = fmt.Fprintf(stdout(), "  auth: %s\n", r.Auth)
	}
	_, _ = fmt.Fprintf(stdout(), "  pid:  %d\n", r.PID)
	return nil
}
