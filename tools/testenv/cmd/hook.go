package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/smazurov/videonode/tools/testenv/internal/envctl"
)

// HookCmd is the parent for hook subcommands. Each maps to a Claude
// Code hook event and is invoked by the settings.json entries that
// `testenv install` writes.
type HookCmd struct {
	SessionStart   HookSessionStartCmd   `cmd:"session-start" help:"SessionStart hook: reap + inject inventory context."`
	SessionEnd     HookSessionEndCmd     `cmd:"session-end" help:"SessionEnd hook: release session envs."`
	PreToolUse     HookPreToolUseCmd     `cmd:"pre-tool-use" help:"PreToolUse hook: steer Claude toward testenv."`
	WorktreeRemove HookWorktreeRemoveCmd `cmd:"worktree-remove" help:"WorktreeRemove hook: release envs from deleted worktree."`
}

type HookSessionStartCmd struct{}

func (c *HookSessionStartCmd) Run(ctx *Context) error {
	// Reap stale envs.
	r, err := envctl.Reap(ctx.Ctx, ctx.StatePath)
	if err != nil {
		return err
	}
	if len(r.Released) > 0 {
		fmt.Fprintf(os.Stderr, "testenv: reaped %d stale env(s)\n", len(r.Released))
	}
	// Inject inventory as context (stdout goes to Claude's context).
	text := envctl.SessionStartContext(ctx.Ctx, ctx.StatePath)
	if text != "" {
		fmt.Print(text)
	}
	return nil
}

type HookSessionEndCmd struct{}

func (c *HookSessionEndCmd) Run(ctx *Context) error {
	if ctx.SessionID == "" {
		return nil
	}
	released, err := envctl.ReleaseSession(ctx.Ctx, ctx.StatePath, ctx.SessionID)
	if err != nil {
		return err
	}
	if len(released) > 0 {
		fmt.Fprintf(os.Stderr, "testenv: released %d env(s) for session %s\n", len(released), ctx.SessionID)
	}
	return nil
}

type HookPreToolUseCmd struct{}

func (c *HookPreToolUseCmd) Run(_ *Context) error {
	d := envctl.EvalPreToolUse(os.Stdin)
	if d.Message != "" {
		fmt.Fprint(os.Stderr, d.Message)
	}
	if d.Block {
		os.Exit(2)
	}
	return nil
}

type HookWorktreeRemoveCmd struct{}

func (c *HookWorktreeRemoveCmd) Run(ctx *Context) error {
	// Read the worktree_dir from the hook payload.
	var payload struct {
		WorktreeDir string `json:"worktree_dir"`
	}
	if err := readJSON(os.Stdin, &payload); err != nil || payload.WorktreeDir == "" {
		return nil
	}
	released, err := envctl.ReleaseWorktree(ctx.Ctx, ctx.StatePath, payload.WorktreeDir)
	if err != nil {
		return err
	}
	if len(released) > 0 {
		fmt.Fprintf(os.Stderr, "testenv: released %d env(s) from removed worktree %s\n",
			len(released), payload.WorktreeDir)
	}
	return nil
}

func readJSON(r *os.File, v any) error {
	dec := json.NewDecoder(r)
	return dec.Decode(v)
}
