package cmd

import (
	"fmt"

	"github.com/smazurov/videonode/tools/testenv/internal/envctl"
)

type ReapCmd struct{}

func (c *ReapCmd) Run(ctx *Context) error {
	r, err := envctl.Reap(ctx.Ctx, ctx.StatePath)
	if err != nil {
		return err
	}
	if len(r.Released) == 0 {
		fmt.Fprintln(stdout(), "no stale envs to reap")
		return nil
	}
	fmt.Fprintf(stdout(), "reaped %d env(s): %v\n", len(r.Released), r.Released)
	return nil
}
