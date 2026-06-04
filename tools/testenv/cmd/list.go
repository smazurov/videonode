package cmd

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"text/tabwriter"
	"time"

	"github.com/smazurov/videonode/tools/testenv/internal/envctl"
)

type ListCmd struct {
	Mine bool `help:"Only show envs owned by the current session."`
}

func truncateLease(id string, max int) string {
	if len(id) <= max {
		return id
	}
	return id[:max-1] + "…"
}

func probeHealth(envs []envctl.EnvInfo) map[string]string {
	result := make(map[string]string, len(envs))
	var mu sync.Mutex
	var wg sync.WaitGroup
	client := &http.Client{Timeout: 2 * time.Second}
	for _, e := range envs {
		wg.Add(1)
		go func(e envctl.EnvInfo) {
			defer wg.Done()
			status := "--"
			if e.HealthURL != "" {
				status = "down"
				req, err := http.NewRequest(http.MethodGet, e.HealthURL, nil)
				if err == nil {
					if e.HealthAuth != "" {
						parts := strings.SplitN(e.HealthAuth, ":", 2)
						if len(parts) == 2 {
							req.SetBasicAuth(parts[0], parts[1])
						}
					}
					resp, doErr := client.Do(req)
					if doErr == nil {
						resp.Body.Close()
						if resp.StatusCode == http.StatusOK {
							status = "ok"
						}
					}
				}
			}
			mu.Lock()
			result[e.ID] = status
			mu.Unlock()
		}(e)
	}
	wg.Wait()
	return result
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
	health := probeHealth(envs)

	w := tabwriter.NewWriter(stdout(), 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tSTATUS\tURL\tWORKTREE\tPID\tAGE")
	for _, e := range envs {
		age := time.Since(e.CreatedAt).Round(time.Second)
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%d\t%s\n",
			e.ID, health[e.ID], e.HTTPURL, e.Worktree, e.PID, age)
		for _, l := range e.Leases {
			fmt.Fprintf(w, "\t\t└ %s\t\t\t\n", truncateLease(l, 30))
		}
	}
	return w.Flush()
}
