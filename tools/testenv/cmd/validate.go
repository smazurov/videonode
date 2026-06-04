package cmd

import (
	"fmt"

	"github.com/smazurov/videonode/tools/testenv/internal/envctl"
)

// ValidateCmd checks the .testenv.toml config in the current directory.
type ValidateCmd struct{}

// Run executes the validate subcommand.
func (c *ValidateCmd) Run(_ *Context) error {
	r, err := envctl.Validate(".")
	if err != nil {
		return err
	}
	if r.LocalOverride != "" {
		_, _ = fmt.Fprintf(stdout(), "config valid (with local overrides from %s)\n", r.LocalOverride)
	} else {
		_, _ = fmt.Fprintln(stdout(), "config valid")
	}
	return nil
}
