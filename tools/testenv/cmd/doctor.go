package cmd

import (
	"fmt"
	"os"

	"github.com/smazurov/videonode/tools/testenv/internal/envctl"
)

// DoctorCmd prints diagnostic information about the testenv installation and current session.
type DoctorCmd struct{}

// Run executes the doctor command, printing version and environment state.
func (c *DoctorCmd) Run(ctx *Context) error {
	w := stdout()

	v := Version
	if v == "" {
		v = vcsRevision()
	}
	if v == "" {
		v = "dev"
	}
	_, _ = fmt.Fprintf(w, "%-14s%s\n", "version:", v)

	session := ctx.SessionID
	if session == "" {
		session = os.Getenv("CLAUDE_SESSION_ID")
	}

	for _, f := range envctl.Doctor(ctx.StatePath, session) {
		_, _ = fmt.Fprintf(w, "%-14s%s\n", f.Key+":", f.Value)
	}
	return nil
}
