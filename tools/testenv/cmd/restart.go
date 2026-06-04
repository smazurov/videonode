package cmd

import (
	"fmt"

	"github.com/smazurov/videonode/tools/testenv/internal/envctl"
)

type RestartCmd struct {
	EnvID string `arg:"" optional:"" help:"Env id to restart. Defaults to the current session's env."`
}

func (c *RestartCmd) Run(ctx *Context) error {
	r, err := envctl.Restart(ctx.Ctx, envctl.RestartParams{
		StatePath: ctx.StatePath,
		EnvID:     c.EnvID,
		Session:   ctx.SessionID,
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout(), "env %s restarted\n", r.EnvID)
	fmt.Fprintf(stdout(), "  url:  %s\n", r.HTTPURL)
	if r.Auth != "" {
		fmt.Fprintf(stdout(), "  auth: %s\n", r.Auth)
	}
	fmt.Fprintf(stdout(), "  pid:  %d\n", r.PID)
	return nil
}
