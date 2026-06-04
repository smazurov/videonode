package cmd

import (
	"fmt"
	"strings"

	"github.com/smazurov/videonode/tools/testenv/internal/envctl"
)

// UpCmd starts a new test environment and prints its URL.
type UpCmd struct {
	Lock []string `name:"lock" short:"l" help:"Exclusive resource lock(s), e.g. device:/dev/video0. Repeatable."`
}

// Run executes the up subcommand.
func (c *UpCmd) Run(ctx *Context) error {
	r, err := envctl.Up(ctx.Ctx, envctl.UpParams{
		StatePath: ctx.StatePath,
		Session:   ctx.SessionID,
		Locks:     c.Lock,
	})
	if err != nil {
		return err
	}
	var portLines []string
	for name, port := range r.Ports {
		portLines = append(portLines, fmt.Sprintf("  %s: %d", name, port))
	}
	_, _ = fmt.Fprintf(stdout(), "env %s up · slot %d\n", r.EnvID, r.Slot)
	_, _ = fmt.Fprintf(stdout(), "  url:  %s\n", r.HTTPURL)
	if r.Auth != "" {
		_, _ = fmt.Fprintf(stdout(), "  auth: %s\n", r.Auth)
	}
	for _, line := range portLines {
		_, _ = fmt.Fprintln(stdout(), line)
	}
	if r.LocalOverride != "" {
		_, _ = fmt.Fprintf(stdout(), "  config: .testenv.toml + %s\n", r.LocalOverride)
	}
	_, _ = fmt.Fprintf(stdout(), "  data: %s\n  pid:  %d\n", r.DataDir, r.PID)
	if len(c.Lock) > 0 {
		_, _ = fmt.Fprintf(stdout(), "  locks: %s\n", strings.Join(c.Lock, ", "))
	}
	return nil
}
