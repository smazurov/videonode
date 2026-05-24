package cmd

import (
	"fmt"

	"github.com/smazurov/videonode/tools/testenv/internal/envctl"
)

type ValidateCmd struct{}

func (c *ValidateCmd) Run(ctx *Context) error {
	if err := envctl.Validate("."); err != nil {
		return err
	}
	fmt.Fprintln(stdout(), "config valid")
	return nil
}
