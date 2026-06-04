// Package cmd holds the testenv CLI subcommands. Each is a thin shell
// over internal/envctl — cmd/ must not import store, slots, spawn, or
// reaper directly (enforced by import_test.go).
package cmd

import (
	"context"
	"io"
	"os"
)

// Context bundles CLI-level state passed to each subcommand's Run.
type Context struct {
	Ctx       context.Context
	StatePath string
	SessionID string
}

var stdoutW io.Writer = os.Stdout

func stdout() io.Writer { return stdoutW }
