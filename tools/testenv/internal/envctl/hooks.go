package envctl

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// HookDecision is the result of evaluating a hook.
type HookDecision struct {
	Block   bool   // exit 2 = block the action
	Message string // shown to Claude (stderr if blocking, stdout context if not)
}

// PreToolUsePayload is the JSON stdin from a PreToolUse hook.
type PreToolUsePayload struct {
	ToolName  string          `json:"tool_name"`
	ToolInput json.RawMessage `json:"tool_input"`
	SessionID string          `json:"session_id"`
	Cwd       string          `json:"cwd"`
}

// BashToolInput is the tool_input shape for Bash tool calls.
type BashToolInput struct {
	Command string `json:"command"`
}

// EvalPreToolUse reads the hook payload from stdin and decides whether
// to block the action. All pattern-matching rules live here — this is
// the single source of truth for "what should Claude not do manually."
//
// Rules are grounded in real transcript evidence:
//   - Pattern 1+2+3 (41 hits): manual daemon spawn/kill
//   - Pattern 5 (60+ hits): curl to hardcoded ports without testenv
//   - Pattern 6 (5 hits): cmake --install from worktree
func EvalPreToolUse(r io.Reader) HookDecision {
	var payload PreToolUsePayload
	if err := json.NewDecoder(r).Decode(&payload); err != nil {
		return HookDecision{} // can't parse → allow
	}
	if payload.ToolName != "Bash" {
		return HookDecision{}
	}
	var bash BashToolInput
	if err := json.Unmarshal(payload.ToolInput, &bash); err != nil {
		return HookDecision{}
	}
	cmd := bash.Command
	return evalBashCommand(cmd, payload.Cwd)
}

func evalBashCommand(cmd, cwd string) HookDecision {
	// Pattern 1+2: manual daemon spawn.
	// Transcript examples:
	//   ./videonode --config /tmp/vntest/config.toml > /tmp/vntest/daemon.log 2>&1 &
	//   nohup ./tmp/main > /tmp/videonode-daemon.log 2>&1 &
	//   /home/stepan/.claude/jobs/.../videonode -p :8099 ...
	if isManualDaemonSpawn(cmd) {
		return HookDecision{
			Block:   true,
			Message: "Use `testenv up` instead of spawning videonode manually — it allocates a clean port slot, registers in the inventory, and other sessions can see it via `testenv list`.",
		}
	}

	// Pattern 3: manual daemon kill.
	// Transcript examples:
	//   pkill -f videonode-source
	//   kill 3839890
	if isManualDaemonKill(cmd) {
		return HookDecision{
			Block:   false, // warn, don't block — might be intentional debugging
			Message: "Consider using `testenv down` to cleanly release the env, slot, and leases. Manual kill leaves stale registry entries until the next reap.",
		}
	}

	// Pattern 6: cmake --install from a worktree.
	// Transcript example:
	//   cmake --install .../worktrees/nv12-stale-mmap-test/composer/build/dev
	if strings.Contains(cmd, "cmake --install") && strings.Contains(cwd, ".claude/worktrees/") {
		return HookDecision{
			Block:   true,
			Message: "Don't install native binaries to ~/.local/bin from a worktree — testenv spawns with the worktree's local build via NATIVE_PIPELINE_* env vars. Installing from here overwrites the shared path and affects other sessions.",
		}
	}

	return HookDecision{}
}

func isManualDaemonSpawn(cmd string) bool {
	// Must look like a daemon launch, not a build or test.
	// Key signals: running the videonode binary with port flags, backgrounding,
	// or nohup — but NOT `go build`, `go test`, `go run . openapi`, `testenv`.
	if strings.Contains(cmd, "go build") || strings.Contains(cmd, "go test") ||
		strings.Contains(cmd, "testenv") || strings.Contains(cmd, "go install") {
		return false
	}
	// Direct binary execution with daemon-like patterns.
	hasVideonode := strings.Contains(cmd, "./videonode") ||
		strings.Contains(cmd, "./tmp/main") ||
		strings.Contains(cmd, "/videonode ")
	hasDaemonSignal := strings.HasSuffix(strings.TrimSpace(cmd), "&") ||
		strings.Contains(cmd, "nohup") ||
		strings.Contains(cmd, "> /tmp/") ||
		strings.Contains(cmd, "2>&1 &") ||
		strings.Contains(cmd, "SERVER_PORT") ||
		strings.Contains(cmd, "STREAMING_RTSP_PORT") ||
		strings.Contains(cmd, "-p :") ||
		strings.Contains(cmd, "--srt-addr") ||
		strings.Contains(cmd, "--streaming-rtsp-port")
	return hasVideonode && hasDaemonSignal
}

func isManualDaemonKill(cmd string) bool {
	if strings.Contains(cmd, "testenv") {
		return false
	}
	hasKill := strings.Contains(cmd, "pkill") || strings.Contains(cmd, "killall") ||
		(strings.Contains(cmd, "kill ") && !strings.Contains(cmd, "kill -0"))
	hasTarget := strings.Contains(cmd, "videonode") || strings.Contains(cmd, "ffmpeg")
	return hasKill && hasTarget
}

// PostToolUsePayload is the JSON stdin from a PostToolUse hook.
type PostToolUsePayload struct {
	ToolName  string `json:"tool_name"`
	SessionID string `json:"session_id"`
	Cwd       string `json:"cwd"`
}

// EvalPostToolUse handles the PostToolUse hook. If the tool was
// EnterWorktree, it registers the session's new cwd so the MCP
// server can resolve the worktree path.
func EvalPostToolUse(r io.Reader, statePath string) HookDecision {
	var payload PostToolUsePayload
	if err := json.NewDecoder(r).Decode(&payload); err != nil {
		return HookDecision{}
	}
	if payload.ToolName != "EnterWorktree" {
		return HookDecision{}
	}
	if payload.SessionID == "" || payload.Cwd == "" {
		return HookDecision{}
	}
	_ = RegisterSession(statePath, payload.SessionID, payload.Cwd)
	return HookDecision{}
}

// SessionStartPayload is the JSON stdin from a SessionStart hook.
type SessionStartPayload struct {
	SessionID string `json:"session_id"`
	Cwd       string `json:"cwd"`
}

// EvalSessionStart registers the session's initial cwd so that
// resolveWorktree can find it even before EnterWorktree fires.
func EvalSessionStart(r io.Reader, statePath string) {
	var payload SessionStartPayload
	if err := json.NewDecoder(r).Decode(&payload); err != nil {
		return
	}
	if payload.SessionID == "" || payload.Cwd == "" {
		return
	}
	_ = RegisterSession(statePath, payload.SessionID, payload.Cwd)
}

// SessionStartContext returns inventory text to inject into the
// session as additional context.
func SessionStartContext(ctx context.Context, statePath string) string {
	envs, err := List(ctx, ListParams{StatePath: statePath})
	if err != nil || len(envs) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("Active testenv environments:\n")
	for _, e := range envs {
		_, _ = fmt.Fprintf(&b, "  slot %d: %s (%s/%s) at %s [worktree=%s pid=%d]",
			e.Slot, e.ID, e.Target, e.Source, e.HTTPURL, e.Worktree, e.PID)
		if len(e.Leases) > 0 {
			_, _ = fmt.Fprintf(&b, " holds %s", strings.Join(e.Leases, ", "))
		}
		b.WriteString("\n")
	}
	b.WriteString("Use `testenv list` to refresh. Use `testenv up` to create a new env.\n")
	return b.String()
}
