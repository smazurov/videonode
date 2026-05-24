// Package cmd holds the testenv subcommand implementations. Each
// subcommand is a Kong struct with a Run method that receives the
// shared *Context.
package cmd

import (
	"context"
	"fmt"

	"github.com/smazurov/videonode/tools/testenv/internal/reaper"
	"github.com/smazurov/videonode/tools/testenv/internal/store"
)

// Context bundles everything the subcommands need.
type Context struct {
	Ctx       context.Context
	StatePath string // optional override; "" uses store.DefaultPath().
	SessionID string // CLAUDE_SESSION_ID at time of invocation, may be empty.
}

// OpenStore opens the configured store path.
func (c *Context) OpenStore() (*store.Store, error) {
	path := c.StatePath
	if path == "" {
		path = store.DefaultPath()
	}
	return store.Open(path)
}

// ReapBefore runs a reap sweep and discards the released list. Called
// at the top of mutating subcommands so stale records can't shadow a
// fresh allocation. Errors are non-fatal and printed to stderr.
func ReapBefore(s *store.Store) {
	if _, err := reaper.Reap(s); err != nil {
		fmt.Fprintf(stderr(), "warn: reap before action failed: %v\n", err)
	}
}
