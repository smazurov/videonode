// Command testenv coordinates shared test environments across
// parallel Claude Code sessions: predictable port slots, device
// leases, dead-PID reaping, and a single inventory queryable from
// any worktree.
//
// See the plan at /home/stepan/.claude/plans/lets-plan-this-cli-lazy-sutton.md
// for the design.
package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/alecthomas/kong"

	"github.com/smazurov/videonode/tools/testenv/cmd"
	"github.com/smazurov/videonode/tools/testenv/internal/config"
)

// cli is the Kong root.
type cli struct {
	StatePath string `help:"Override the SQLite state path." env:"TESTENV_STATE"`
	Session   string `help:"Override the owning session id (defaults to $CLAUDE_SESSION_ID)." env:"CLAUDE_SESSION_ID"`

	Init           cmd.InitCmd           `cmd:"" help:"Create a .testenv.toml template."`
	Up             cmd.UpCmd             `cmd:"" help:"Spin up a test environment."`
	Down           cmd.DownCmd           `cmd:"" help:"Tear down a test environment."`
	Restart        cmd.RestartCmd        `cmd:"" help:"Rebuild and restart a test environment."`
	List           cmd.ListCmd           `cmd:"" help:"Show the active env inventory."`
	Lease          cmd.LeaseCmd          `cmd:"" help:"Acquire a named resource lease."`
	Release        cmd.ReleaseCmd        `cmd:"" help:"Release a named resource lease."`
	ReleaseSession cmd.ReleaseSessionCmd `cmd:"release-session" help:"Release everything a session owns."`
	Reap           cmd.ReapCmd           `cmd:"" help:"Sweep stale env records."`
	MCP            cmd.MCPCmd            `cmd:"" help:"Run as a stdio MCP server."`
	Validate       cmd.ValidateCmd       `cmd:"" help:"Validate the .testenv.toml config."`
	Install        cmd.InstallCmd        `cmd:"" help:"Write skills, hooks, and .mcp.json into a project."`
	Hook           cmd.HookCmd           `cmd:"" help:"Hook handlers invoked by Claude Code settings.json."`
	Version        cmd.VersionCmd        `cmd:"" help:"Print version + git commit captured at build."`
}

func main() {
	var root cli
	k := kong.Parse(&root,
		kong.Name("testenv"),
		kong.Description("Coordinator for parallel test environments."),
		kong.UsageOnError(),
	)
	if root.StatePath == "" {
		root.StatePath = resolveProjectStatePath()
	}
	err := k.Run(&cmd.Context{
		Ctx:       context.Background(),
		StatePath: root.StatePath,
		SessionID: root.Session,
	})
	if err != nil {
		_, _ = os.Stderr.WriteString("testenv: " + err.Error() + "\n")
		os.Exit(1)
	}
}

func resolveProjectStatePath() string {
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, config.FileName)); err == nil {
			project := filepath.Base(resolveProjectRoot(dir))
			stateDir := os.Getenv("XDG_STATE_HOME")
			if stateDir == "" {
				home, _ := os.UserHomeDir()
				stateDir = filepath.Join(home, ".local", "state")
			}
			return filepath.Join(stateDir, "testenv", project, "state.db")
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

func resolveProjectRoot(dir string) string {
	const marker = "/.claude/worktrees/"
	if i := strings.Index(dir, marker); i >= 0 {
		return dir[:i]
	}
	return dir
}
