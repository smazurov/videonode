package cmd

import (
	"fmt"
	"text/tabwriter"
	"time"

	"github.com/smazurov/videonode/tools/testenv/internal/envctl"
)

type ListCmd struct {
	Mine bool `help:"Only show envs owned by the current session."`
}

func (c *ListCmd) Run(ctx *Context) error {
	envs, err := envctl.List(ctx.Ctx, envctl.ListParams{
		StatePath: ctx.StatePath,
		Mine:      c.Mine,
		Session:   ctx.SessionID,
	})
	if err != nil {
		return err
	}
	if len(envs) == 0 {
		if c.Mine {
			fmt.Fprintln(stdout(), "no envs owned by current session")
		} else {
			fmt.Fprintln(stdout(), "no envs registered")
		}
		return nil
	}
	w := tabwriter.NewWriter(stdout(), 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "SLOT\tID\tTARGET\tSOURCE\tURL\tWORKTREE\tPID\tAGE")
	for _, e := range envs {
		age := time.Since(e.CreatedAt).Round(time.Second)
		fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\t%s\t%d\t%s\n",
			e.Slot, e.ID, e.Target, e.Source, e.HTTPURL,
			e.Worktree, e.PID, age)
		for _, l := range e.Leases {
			fmt.Fprintf(w, "  \t  holds\t%s\t\t\t\t\t\n", l)
		}
	}
	return w.Flush()
}
