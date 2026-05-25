package cmd

import (
	"fmt"

	"github.com/smazurov/videonode/tools/testenv/internal/envctl"
)

type ValidateCmd struct{}

func (c *ValidateCmd) Run(_ *Context) error {
	r, err := envctl.Validate(".")
	if err != nil {
		return err
	}
	if r.LocalOverride != "" {
		fmt.Fprintf(stdout(), "config valid (with local overrides from %s)\n", r.LocalOverride)
	} else {
		fmt.Fprintln(stdout(), "config valid")
	}
	return nil
}
