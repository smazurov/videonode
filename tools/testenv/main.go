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

	"github.com/alecthomas/kong"

	"github.com/smazurov/videonode/tools/testenv/cmd"
)

// cli is the Kong root.
type cli struct {
	StatePath string `help:"Override the SQLite state path." env:"TESTENV_STATE"`
	Session   string `help:"Override the owning session id (defaults to $CLAUDE_SESSION_ID)." env:"CLAUDE_SESSION_ID"`

	Up             cmd.UpCmd             `cmd:"" help:"Spin up a test environment."`
	Down           cmd.DownCmd           `cmd:"" help:"Tear down a test environment."`
	List           cmd.ListCmd           `cmd:"" help:"Show the active env inventory."`
	Lease          cmd.LeaseCmd          `cmd:"" help:"Acquire a named resource lease."`
	Release        cmd.ReleaseCmd        `cmd:"" help:"Release a named resource lease."`
	ReleaseSession cmd.ReleaseSessionCmd `cmd:"release-session" help:"Release everything a session owns."`
	Reap           cmd.ReapCmd           `cmd:"" help:"Sweep stale env records."`
	MCP            cmd.MCPCmd            `cmd:"" help:"Run as a stdio MCP server."`
	Install        cmd.InstallCmd        `cmd:"" help:"Write skills, hooks, and .mcp.json into a project."`
	Hook           cmd.HookCmd           `cmd:"" help:"Hook handlers invoked by Claude Code settings.json."`
	Version        cmd.VersionCmd        `cmd:"" help:"Print version + git commit captured at build."`
}

func main() {
	var root cli
	k := kong.Parse(&root,
		kong.Name("testenv"),
		kong.Description("Coordinator for parallel videonode test environments."),
		kong.UsageOnError(),
	)
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
