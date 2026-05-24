// Package mcpsrv registers the testenv MCP tools on a mcp.Server.
// Each tool is a thin dispatcher to internal/envctl — mcpsrv must not
// import store, slots, spawn, or reaper directly (enforced by
// import_test.go).
package mcpsrv

import (
	"context"
	"fmt"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/smazurov/videonode/tools/testenv/internal/envctl"
)

// Register adds all testenv tools to server, backed by statePath.
func Register(server *mcp.Server, statePath string) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "testenv_up",
		Description: "Spin up a videonode test environment. Returns URLs on the allocated slot.",
	}, upHandler(statePath))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "testenv_down",
		Description: "Tear down a test environment by env_id (or the current session's).",
	}, downHandler(statePath))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "testenv_list",
		Description: "Show the active test-env inventory across all sessions.",
	}, listHandler(statePath))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "testenv_lease",
		Description: "Acquire a named resource lease for the current session's env.",
	}, leaseHandler(statePath))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "testenv_release",
		Description: "Release a named resource lease.",
	}, releaseHandler(statePath))
}

// --- input / output structs (JSON schema derived by the SDK) ---

type upIn struct {
	Target  string `json:"target,omitempty"`
	Source  string `json:"source,omitempty"`
	Device  string `json:"device,omitempty"`
	Session string `json:"session,omitempty"`
}

type upOut struct {
	EnvID   string `json:"env_id"`
	Slot    int    `json:"slot"`
	HTTPURL string `json:"http_url"`
	RTSPURL string `json:"rtsp_url"`
	SRTURL  string `json:"srt_url"`
	PID     int    `json:"pid"`
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
	Target    string   `json:"target"`
	Source    string   `json:"source_mode"`
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

// --- handlers (each is a one-liner to envctl) ---

func sessionOrEnv(s string) string {
	if s != "" {
		return s
	}
	return os.Getenv("CLAUDE_SESSION_ID")
}

func upHandler(sp string) mcp.ToolHandlerFor[upIn, upOut] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in upIn) (*mcp.CallToolResult, upOut, error) {
		r, err := envctl.Up(ctx, envctl.UpParams{
			StatePath: sp, Session: sessionOrEnv(in.Session),
			Target: in.Target, Source: in.Source, Device: in.Device,
		})
		if err != nil {
			return nil, upOut{}, err
		}
		return nil, upOut{
			EnvID: r.EnvID, Slot: r.Slot,
			HTTPURL: r.HTTPURL, RTSPURL: r.RTSPURL, SRTURL: r.SRTURL,
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
				EnvID: e.ID, Slot: e.Slot, Target: e.Target,
				Source: e.Source, HTTPURL: e.HTTPURL,
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

func releaseHandler(sp string) mcp.ToolHandlerFor[leaseIn, leaseOut] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in leaseIn) (*mcp.CallToolResult, leaseOut, error) {
		err := envctl.Release(ctx, sp, in.ResourceID)
		if err != nil {
			return nil, leaseOut{}, err
		}
		return nil, leaseOut{ResourceID: in.ResourceID}, nil
	}
}

// Unused but keeps the import for format consistency.
var _ = fmt.Sprintf
