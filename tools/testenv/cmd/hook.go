package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"

	"github.com/smazurov/videonode/tools/testenv/internal/envctl"
)

type HookCmd struct {
	SessionStart   HookSessionStartCmd   `cmd:"session-start" help:"SessionStart hook: reap + inject inventory context."`
	SessionEnd     HookSessionEndCmd     `cmd:"session-end" help:"SessionEnd hook: release session envs."`
	PreToolUse     HookPreToolUseCmd     `cmd:"pre-tool-use" help:"PreToolUse hook: steer Claude toward testenv."`
	PostToolUse    HookPostToolUseCmd    `cmd:"post-tool-use" help:"PostToolUse hook: track worktree on EnterWorktree."`
	WorktreeRemove HookWorktreeRemoveCmd `cmd:"worktree-remove" help:"WorktreeRemove hook: release envs from deleted worktree."`
}

type HookSessionStartCmd struct{}

func (c *HookSessionStartCmd) Run(ctx *Context) error {
	r, err := envctl.Reap(ctx.Ctx, ctx.StatePath)
	if err != nil {
		return err
	}
	if len(r.Released) > 0 {
		fmt.Fprintf(os.Stderr, "testenv: reaped %d stale env(s)\n", len(r.Released))
	}
	text := envctl.SessionStartContext(ctx.Ctx, ctx.StatePath)
	if text != "" {
		fmt.Print(text)
	}
	active := 0
	if envs, err := envctl.List(ctx.Ctx, envctl.ListParams{StatePath: ctx.StatePath}); err == nil {
		active = len(envs)
	}
	hookLog(ctx.StatePath, "session-start", ctx.SessionID,
		"reaped", strconv.Itoa(len(r.Released)),
		"active", strconv.Itoa(active))
	return nil
}

type HookSessionEndCmd struct{}

func (c *HookSessionEndCmd) Run(ctx *Context) error {
	if ctx.SessionID == "" {
		hookLog(ctx.StatePath, "session-end", "", "released", "0")
		return nil
	}
	released, err := envctl.ReleaseSession(ctx.Ctx, ctx.StatePath, ctx.SessionID)
	if err != nil {
		return err
	}
	if len(released) > 0 {
		fmt.Fprintf(os.Stderr, "testenv: released %d env(s) for session %s\n", len(released), ctx.SessionID)
	}
	hookLog(ctx.StatePath, "session-end", ctx.SessionID,
		"released", strconv.Itoa(len(released)))
	return nil
}

type HookPreToolUseCmd struct{}

func (c *HookPreToolUseCmd) Run(ctx *Context) error {
	raw, _ := io.ReadAll(os.Stdin)
	d := envctl.EvalPreToolUse(bytes.NewReader(raw))

	var toolName string
	var p struct {
		ToolName string `json:"tool_name"`
	}
	if json.Unmarshal(raw, &p) == nil {
		toolName = p.ToolName
	}

	action := "allow"
	if d.Block {
		action = "block"
	}
	kv := []string{"tool", toolName, "action", action}
	if d.Message != "" {
		fmt.Fprint(os.Stderr, d.Message)
		if d.Block {
			kv = append(kv, "reason", d.Message)
		}
	}

	hookLog(ctx.StatePath, "pre-tool-use", ctx.SessionID, kv...)
	if d.Block {
		os.Exit(2)
	}
	return nil
}

type HookPostToolUseCmd struct{}

func (c *HookPostToolUseCmd) Run(ctx *Context) error {
	raw, _ := io.ReadAll(os.Stdin)
	envctl.EvalPostToolUse(bytes.NewReader(raw), ctx.StatePath)

	var toolName string
	var p struct {
		ToolName string `json:"tool_name"`
	}
	if json.Unmarshal(raw, &p) == nil {
		toolName = p.ToolName
	}

	hookLog(ctx.StatePath, "post-tool-use", ctx.SessionID, "tool", toolName)
	return nil
}

type HookWorktreeRemoveCmd struct{}

func (c *HookWorktreeRemoveCmd) Run(ctx *Context) error {
	var payload struct {
		WorktreePath string `json:"worktree_path"`
	}
	if err := readJSON(os.Stdin, &payload); err != nil || payload.WorktreePath == "" {
		hookLog(ctx.StatePath, "worktree-remove", ctx.SessionID, "err", "empty payload")
		return nil
	}
	released, err := envctl.ReleaseWorktree(ctx.Ctx, ctx.StatePath, payload.WorktreePath)
	if err != nil {
		hookLog(ctx.StatePath, "worktree-remove", ctx.SessionID, "path", payload.WorktreePath, "err", err.Error())
		return err
	}
	if len(released) > 0 {
		fmt.Fprintf(os.Stderr, "testenv: released %d env(s) from removed worktree %s\n",
			len(released), payload.WorktreePath)
	}
	hookLog(ctx.StatePath, "worktree-remove", ctx.SessionID,
		"path", payload.WorktreePath,
		"released", strconv.Itoa(len(released)))
	return nil
}

func readJSON(r *os.File, v any) error {
	dec := json.NewDecoder(r)
	return dec.Decode(v)
}
