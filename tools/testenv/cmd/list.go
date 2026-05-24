package cmd

import (
	"fmt"
	"path/filepath"
	"text/tabwriter"
	"time"
)

// ListCmd prints the current env inventory.
type ListCmd struct {
	Mine bool `help:"Only show envs owned by the current session."`
}

func (c *ListCmd) Run(ctx *Context) error {
	s, err := ctx.OpenStore()
	if err != nil {
		return err
	}
	defer s.Close()

	ReapBefore(s)

	envs, err := s.ListEnvs()
	if err != nil {
		return err
	}

	if c.Mine {
		session := ctx.SessionID
		filtered := envs[:0]
		for _, e := range envs {
			if e.OwnerSession == session {
				filtered = append(filtered, e)
			}
		}
		envs = filtered
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
			e.Slot, e.ID, e.Target, e.SourceMode, e.HTTPURL,
			filepath.Base(e.OwnerWorktree), e.OwnerPID, age)

		leases, _ := s.ListLeasesFor(e.ID)
		for _, l := range leases {
			fmt.Fprintf(w, "  \t  holds\t%s\t\t\t\t\t\n", l.ResourceID)
		}
	}
	return w.Flush()
}
