package cmd

import (
	"fmt"

	"github.com/smazurov/videonode/tools/testenv/internal/reaper"
)

// ReapCmd runs a one-shot stale-PID sweep.
type ReapCmd struct{}

func (c *ReapCmd) Run(ctx *Context) error {
	s, err := ctx.OpenStore()
	if err != nil {
		return err
	}
	defer s.Close()
	released, err := reaper.Reap(s)
	if err != nil {
		return err
	}
	if len(released) == 0 {
		fmt.Fprintln(stdout(), "no stale envs to reap")
		return nil
	}
	fmt.Fprintf(stdout(), "reaped %d env(s): %v\n", len(released), released)
	return nil
}
