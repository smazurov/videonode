# testenv — coordinator for parallel videonode test environments

This is a **standalone Go module** (own `go.mod`) nested inside the videonode repo at `tools/testenv/`. It does NOT import videonode internals and must stay that way — keep `go.mod`'s `require` block free of `github.com/smazurov/videonode` (without the `tools/testenv` suffix).

## Why it exists

Multiple parallel Claude Code sessions (worktrees under `.claude/worktrees/`) test videonode changes concurrently. Without coordination they trample each other: same TCP ports, same `/dev/video0`, same per-user binary install path, same SBC. testenv is the registry that hands out predictable port slots, manages device leases with attribution on conflict, auto-reaps dead-session leases via opportunistic PID-alive sweeps, and exposes one CLI + one stdio MCP server backed by the same SQLite store so any session can see what's running where.

Full design rationale in `/home/stepan/.claude/plans/lets-plan-this-cli-lazy-sutton.md`.

## Install

```bash
cd tools/testenv
go install -ldflags "-X github.com/smazurov/videonode/tools/testenv/cmd.Version=$(git rev-parse --short HEAD)" .
```

This puts `testenv` into `$GOPATH/bin` (typically `~/go/bin`). Verify with `testenv version` — it should print the commit you just installed from. If it says `dev`, the ldflags were missing.

After installing the binary, self-install the Claude Code integration (skills, hooks, MCP) into the videonode project:

```bash
testenv install --project-dir ~/dev/videonode
```

This writes `.claude/skills/testenv-{up,down,list}/SKILL.md`, merges `SessionStart`/`SessionEnd` hook entries into `.claude/settings.json`, and writes `.mcp.json` registering the testenv MCP server.

**Version freshness check** (run from any videonode worktree):

```bash
testenv version   # shows embedded commit
git rev-parse --short HEAD   # shows current HEAD
# Should match. If not: cd tools/testenv && go install -ldflags ... .
```

Note: Go's `runtime/debug.ReadBuildInfo()` VCS detection doesn't work reliably in git worktrees (reads the main repo's HEAD, not the worktree's). That's why we use `-ldflags` to stamp the version explicitly.

## Build / Run / Test

```bash
cd tools/testenv
go build .                              # produces ./testenv (for dev iteration)
go test ./...                           # unit tests
golangci-lint run ./...                 # lint
go mod tidy                             # after dep changes
```

## CLI surface

```
testenv up      [--target host|sbc] [--source real|fake] [--device /dev/video0]
testenv down    [<env-id>]                     # default: current session's env
testenv list    [--mine]
testenv lease   <resource-id>                  # e.g. device:/dev/video0
testenv release <resource-id>
testenv release-session [<session-id>]         # default: $CLAUDE_SESSION_ID
testenv reap                                   # one-shot stale-PID sweep
testenv mcp                                    # stdio MCP server
testenv install [--project-dir <path>] [--dry-run]
```

Every mutating subcommand reaps first (cheap; ~5ms). The `reap` and `release-session` subcommands are also fired by the hooks installed via `testenv install`.

## State

Single SQLite file at `$XDG_STATE_HOME/testenv/state.db` (defaulting to `~/.local/state/testenv/state.db`). Schema lives in `internal/store/store.go`; cross-process write paths take an advisory `gofrs/flock` on `state.db.lock` before mutating. WAL mode is enabled. Reboot-survival: stale env rows whose PIDs are no longer alive get reaped on the next invocation of any subcommand.

## Port slots (predictable, not random)

Slots `i ∈ 1..9` map to `http=8090+10i`, `rtsp=8554+10i`, `srt=6001+10i`. Slot 0 is reserved for the canonical air-driven daemon on default ports. The allocator picks the smallest free slot AND verifies bindability with the **bind-and-hold** pattern — open listeners on all three ports while committing the slot row to SQLite, then close them right before exec'ing the daemon (minimizes TOCTOU window between "looks free" and "daemon binds").

## Resource leases

`/dev/video0` is the obvious one. Resource ids are opaque strings (`device:/dev/video0`, `sbc:orangepi5-ultra`, etc.). Conflict is fast-fail with attribution: `error: device:/dev/video0 held by env-3 (worktree=foo pid=12345 since 14:22)`. No queueing in v1. Add new resource conventions sparingly — the string namespace is the API.

## Spawn flow (`testenv up`, host target)

1. Resolve any `[[spawn.binaries]]` overrides declared in `.testenv.toml`. Each entry exports an env var pointing at a worktree-built binary, but only when the file exists; a missing one is left unset (with a loud stderr note) so the daemon falls back to its own default. The tool is binary-agnostic — which env vars and paths to override is entirely a `.testenv.toml` concern.
2. Allocate a slot (bind-and-hold).
3. Acquire any requested device lease, fast-fail with attribution on conflict.
4. Write the `[[spawn.files]]` declared in `.testenv.toml` (e.g. a per-env config), with `${TESTENV_*}` template vars expanded so ids/ports/paths don't collide across envs.
5. Run the `spawn.build` step (if any), then start `spawn.command` with the resolved `[spawn.env]` and `[[spawn.binaries]]` env vars set. cwd = the worktree.
6. Health-poll `spawn.health_url` until 200 (timeout from `spawn.health_timeout`). Return URL.

## Package layout

```
main.go              # Kong root, dispatches to cmd/
cmd/                 # one file per subcommand; each is a Kong struct with Run(*Context) error
internal/store/      # SQLite schema + queries; gofrs/flock for cross-process atomicity
internal/slots/      # port allocator with bind-and-hold
internal/spawn/      # host (and later SBC) bring-up
internal/reaper/     # unix.Kill(pid, 0) sweep
internal/assets/     # //go:embed roots for skills/ and hooks/
internal/assets/skills/{testenv-up,testenv-down,testenv-list}/SKILL.md
internal/assets/hooks/settings.json.tmpl
```

## Dependency contract

**No imports from `github.com/smazurov/videonode/...` (the parent repo).** This module is portable to any project that wants the same coordination model — all project-specific bits (binary paths, env-var names, config-file shapes, post-start commands) live in `.testenv.toml`, not in Go and not in compile-time imports.

Direct deps (see `go.mod` for pins):

- `github.com/alecthomas/kong` — CLI subcommands.
- `modernc.org/sqlite` — pure-Go SQLite, no CGO.
- `github.com/gofrs/flock` — cross-process advisory locks.
- `golang.org/x/sys/unix` — `Kill(pid, 0)` for PID-alive checks.

Planned deps (not yet wired):

- `github.com/modelcontextprotocol/go-sdk` — stdio MCP server.
- `github.com/kevinburke/ssh_config` + `golang.org/x/crypto/ssh` — SBC spawn (`--target sbc`).

## Don't

- Don't hardcode binary paths, env-var names, or config shapes in the tool. They are project concerns and belong in `.testenv.toml` (`[[spawn.binaries]]`, `[spawn.env]`, `[[spawn.files]]`), never in Go. The only rule the tool enforces for binary overrides is: export the override when the file exists, else leave it unset (loud) and let the daemon use its own default.
- Don't add a long-running reaper daemon. Opportunistic reap (on every subcommand entry + SessionStart/SessionEnd hooks) is the contract.
- Don't allocate ports randomly. The slot scheme is intentional — predictability + inventory readability matter more than "use any free port."
- Don't queue on device-lease conflict. Fast-fail with attribution lets the caller decide whether to take over, wait, or pick a different device. Queueing complicates the mental model for no gain at our scale.
- Don't embed videonode internals or proto-generated code. If you find yourself wanting to call into videonode's Go packages, the testenv design has gone wrong — rethink the shape.
