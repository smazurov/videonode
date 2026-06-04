// Package mcpsrv registers the testenv MCP tools on a mcp.Server.
// Each tool is a thin dispatcher to internal/envctl — mcpsrv must not
// import store, slots, spawn, reaper, or config directly (enforced by
// import_test.go).
package mcpsrv

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/smazurov/videonode/tools/testenv/internal/envctl"
)

const resumeEnvKey = "TESTENV_MCP_RESUME"

var (
	selfExe     string
	selfModTime time.Time
	selfOnce    sync.Once
	mcpServer   *mcp.Server

	inflight    atomic.Int64
	reloadArmed atomic.Bool

	// Grace window after the last request drains, giving the SDK time to
	// flush its response to stdout before we replace the process image.
	// Overridable in tests.
	reloadGrace = 200 * time.Millisecond

	// Indirection points so reload behaviour is unit-testable without
	// actually stat'ing the binary or exec'ing the process.
	binaryUpdatedFn = binaryUpdated
	execReloadFn    = execReload

	// Diagnostic logs go to stderr. This must never be os.Stdout — stdout
	// is the JSON-RPC transport for the stdio MCP server, and any stray
	// bytes there corrupt the protocol.
	logSink io.Writer = os.Stderr
)

func logf(format string, args ...any) {
	_, _ = fmt.Fprintf(logSink, "testenv-mcp: "+format+"\n", args...)
}

func initSelf() {
	selfOnce.Do(func() {
		exe, err := os.Executable()
		if err != nil {
			return
		}
		selfExe = exe
		if fi, err := os.Stat(exe); err == nil {
			selfModTime = fi.ModTime()
		}
	})
}

func binaryUpdated() bool {
	initSelf()
	if selfExe == "" {
		return false
	}
	fi, err := os.Stat(selfExe)
	if err != nil {
		return false
	}
	return fi.ModTime().After(selfModTime)
}

// reloadIfUpdated arms a one-shot background hot-reload when the on-disk
// binary is newer than this running process. It MUST be called only after
// a tool handler has produced its result. The previous implementation
// exec'd at the *start* of every handler: if the binary had been rebuilt
// (which happens routinely — parallel sessions reinstall testenv), the
// re-exec replaced the process image mid-request and abandoned the
// in-flight JSON-RPC response, so the MCP client hung forever waiting for
// a reply the new image never sent. We instead let the current response
// flush, wait for all in-flight requests to drain, then exec — so the
// triggering call always completes and no request is ever dropped.
func reloadIfUpdated() {
	if !binaryUpdatedFn() || mcpServer == nil {
		return
	}
	if !reloadArmed.CompareAndSwap(false, true) {
		return
	}
	logf("binary rebuilt on disk; hot-reload armed, draining in-flight requests")
	go func() {
		for inflight.Load() > 0 {
			time.Sleep(20 * time.Millisecond)
		}
		time.Sleep(reloadGrace)
		logf("in-flight requests drained; re-exec'ing %s", selfExe)
		if !execReloadFn() {
			reloadArmed.Store(false) // exec failed — let a later call retry
			logf("hot-reload could not complete; continuing on current image")
		}
	}()
}

// execReload replaces the process image with a fresh copy of the binary,
// passing the current session state so the reconnect is transparent to the
// client. It does not return on success (the image is gone); it returns
// false only when the reload could not be attempted or exec failed.
func execReload() bool {
	var params *mcp.InitializeParams
	for ss := range mcpServer.Sessions() {
		params = ss.InitializeParams()
		break
	}
	if params == nil {
		return false
	}
	data, err := json.Marshal(params)
	if err != nil {
		return false
	}
	env := os.Environ()
	env = append(env, resumeEnvKey+"="+base64.StdEncoding.EncodeToString(data))
	_ = syscall.Exec(selfExe, os.Args, env)
	return false
}

// ResumeState returns saved InitializeParams if this is a hot-reload
// resume, or nil for a cold start. Unsets the env var immediately.
func ResumeState() *mcp.InitializeParams {
	raw := os.Getenv(resumeEnvKey)
	os.Unsetenv(resumeEnvKey)
	if raw == "" {
		return nil
	}
	data, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil
	}
	var params mcp.InitializeParams
	if err := json.Unmarshal(data, &params); err != nil {
		return nil
	}
	return &params
}

// addTool wraps mcp.AddTool with deferred hot-reload middleware: the
// handler runs and responds first, then a reload is armed if the binary
// was rebuilt — never mid-request.
func addTool[In, Out any](s *mcp.Server, t *mcp.Tool, h mcp.ToolHandlerFor[In, Out]) {
	wrapped := func(ctx context.Context, req *mcp.CallToolRequest, in In) (*mcp.CallToolResult, Out, error) {
		inflight.Add(1)
		defer func() {
			inflight.Add(-1)
			reloadIfUpdated()
		}()
		return h(ctx, req, in)
	}
	mcp.AddTool(s, t, mcp.ToolHandlerFor[In, Out](wrapped))
}

// Register adds all testenv tools to server, backed by statePath.
func Register(server *mcp.Server, statePath string) {
	mcpServer = server
	addTool(server, &mcp.Tool{
		Name:        "testenv_up",
		Description: "Spin up a test environment per .testenv.toml (+ .testenv.local.toml overrides). Returns URL, auth, and ports.",
	}, upHandler(statePath))

	addTool(server, &mcp.Tool{
		Name:        "testenv_down",
		Description: "Tear down a test environment by env_id (or the current session's).",
	}, downHandler(statePath))

	addTool(server, &mcp.Tool{
		Name:        "testenv_list",
		Description: "Show the active test-env inventory across all sessions.",
	}, listHandler(statePath))

	addTool(server, &mcp.Tool{
		Name:        "testenv_lease",
		Description: "Acquire an exclusive resource lock for the current session's env.",
	}, leaseHandler(statePath))

	addTool(server, &mcp.Tool{
		Name:        "testenv_restart",
		Description: "Rebuild and restart a test environment's daemon. Picks up config and local overrides.",
	}, restartHandler(statePath))

	addTool(server, &mcp.Tool{
		Name:        "testenv_release",
		Description: "Release an exclusive resource lock.",
	}, releaseHandler(statePath))
}

type upIn struct {
	Locks   []string `json:"locks,omitempty"`
	Session string   `json:"session,omitempty"`
}

type upOut struct {
	EnvID   string         `json:"env_id"`
	Slot    int            `json:"slot"`
	Ports   map[string]int `json:"ports"`
	HTTPURL string         `json:"http_url"`
	Auth    string         `json:"auth,omitempty"`
	PID     int            `json:"pid"`
}

type downIn struct {
	EnvID   string `json:"env_id,omitempty"`
	Session string `json:"session,omitempty"`
}

type downOut struct {
	EnvID string `json:"env_id"`
	PID   int    `json:"pid"`
}

type listIn struct {
	Mine    bool   `json:"mine,omitempty"`
	Session string `json:"session,omitempty"`
}

type envEntry struct {
	EnvID     string   `json:"env_id"`
	Slot      int      `json:"slot"`
	HTTPURL   string   `json:"http_url"`
	Worktree  string   `json:"worktree"`
	PID       int      `json:"pid"`
	Leases    []string `json:"leases,omitempty"`
	CreatedAt string   `json:"created_at"`
}

type listOut struct {
	Envs []envEntry `json:"envs"`
}

type leaseIn struct {
	ResourceID string `json:"resource_id"`
	Session    string `json:"session,omitempty"`
}

type leaseOut struct {
	ResourceID string `json:"resource_id"`
}

func sessionOrEnv(s string) string {
	if s != "" {
		return s
	}
	if v := os.Getenv("CLAUDE_CODE_SESSION_ID"); v != "" {
		return v
	}
	return os.Getenv("CLAUDE_SESSION_ID")
}

func upHandler(sp string) mcp.ToolHandlerFor[upIn, upOut] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in upIn) (*mcp.CallToolResult, upOut, error) {
		r, err := envctl.Up(ctx, envctl.UpParams{
			StatePath: sp, Session: sessionOrEnv(in.Session),
			Locks: in.Locks,
		})
		if err != nil {
			return nil, upOut{}, err
		}
		return nil, upOut{
			EnvID: r.EnvID, Slot: r.Slot,
			Ports: r.Ports, HTTPURL: r.HTTPURL, Auth: r.Auth,
			PID: r.PID,
		}, nil
	}
}

func downHandler(sp string) mcp.ToolHandlerFor[downIn, downOut] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in downIn) (*mcp.CallToolResult, downOut, error) {
		r, err := envctl.Down(ctx, envctl.DownParams{
			StatePath: sp, EnvID: in.EnvID, Session: sessionOrEnv(in.Session),
		})
		if err != nil {
			return nil, downOut{}, err
		}
		return nil, downOut{EnvID: r.EnvID, PID: r.PID}, nil
	}
}

func listHandler(sp string) mcp.ToolHandlerFor[listIn, listOut] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in listIn) (*mcp.CallToolResult, listOut, error) {
		envs, err := envctl.List(ctx, envctl.ListParams{
			StatePath: sp, Mine: in.Mine, Session: sessionOrEnv(in.Session),
		})
		if err != nil {
			return nil, listOut{}, err
		}
		var entries []envEntry
		for _, e := range envs {
			entries = append(entries, envEntry{
				EnvID: e.ID, Slot: e.Slot, HTTPURL: e.HTTPURL,
				Worktree: e.Worktree, PID: e.PID,
				Leases: e.Leases, CreatedAt: e.CreatedAt.Format("15:04:05"),
			})
		}
		return nil, listOut{Envs: entries}, nil
	}
}

func leaseHandler(sp string) mcp.ToolHandlerFor[leaseIn, leaseOut] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in leaseIn) (*mcp.CallToolResult, leaseOut, error) {
		err := envctl.Lease(ctx, envctl.LeaseParams{
			StatePath: sp, Session: sessionOrEnv(in.Session),
			ResourceID: in.ResourceID,
		})
		if err != nil {
			return nil, leaseOut{}, err
		}
		return nil, leaseOut{ResourceID: in.ResourceID}, nil
	}
}

type restartIn struct {
	EnvID   string `json:"env_id,omitempty"`
	Session string `json:"session,omitempty"`
}

type restartOut struct {
	EnvID   string `json:"env_id"`
	HTTPURL string `json:"http_url"`
	Auth    string `json:"auth,omitempty"`
	PID     int    `json:"pid"`
}

func restartHandler(sp string) mcp.ToolHandlerFor[restartIn, restartOut] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in restartIn) (*mcp.CallToolResult, restartOut, error) {
		r, err := envctl.Restart(ctx, envctl.RestartParams{
			StatePath: sp, EnvID: in.EnvID, Session: sessionOrEnv(in.Session),
		})
		if err != nil {
			return nil, restartOut{}, err
		}
		return nil, restartOut{
			EnvID: r.EnvID, HTTPURL: r.HTTPURL, Auth: r.Auth, PID: r.PID,
		}, nil
	}
}

func releaseHandler(sp string) mcp.ToolHandlerFor[leaseIn, leaseOut] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in leaseIn) (*mcp.CallToolResult, leaseOut, error) {
		err := envctl.Release(ctx, sp, in.ResourceID)
		if err != nil {
			return nil, leaseOut{}, err
		}
		return nil, leaseOut{ResourceID: in.ResourceID}, nil
	}
}
