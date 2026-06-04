---
name: deploy-rig
description: Deploy videonode to the RK3588 rig (orangepi5-ultra.lan) — host sanity build, sync + rig-native composer build, UI-embedded arm64 Go supervisor, install into /usr/bin, restart the videonode.service system unit, and verify. Use after non-trivial composer/ (C++) or Go-side changes that need real-hardware verification before they are trusted.
---

# deploy-rig

End-to-end real-hardware deploy + restart for the videonode rig. The deploy
mechanism lives in **load-bearing scripts**, not in this prose — this file tells
you which script to run, how to scope it, and how to keep the scripts honest.

## The scripts ARE the deploy (read this first)

Three scripts at the repo root own the deploy. They are the source of truth;
do **not** hand-type the equivalent ssh/scp/sudo sequences inline.

| script | what it does | safe to run alone |
| --- | --- | --- |
| `scripts/deploy-preflight.sh` | one ssh: prints `local`/`service`/`go`/`cpp` versions + a `decision:` line | yes (read-only) |
| `scripts/build-go-arm64.sh` | UI `pnpm build` → `-tags ui_embed` + version ldflags → arm64 cross-build → stage to rig `/tmp` | yes |
| `scripts/deploy-rig.sh` | orchestrator: preflight → host build → stop → composer sync+build → go build → install `/usr/bin` → start → preflight | yes (this is the whole deploy) |

**Maintenance contract — before driving a deploy, open the script you are about
to run and confirm its embedded rig contract still matches reality.** These
scripts hard-code the current truth (system unit, `/usr/bin`, `sudo -S`, the
`/home/orangepi/composer/build/src/bin` and `/tmp/...staging` paths, the module
path in the ldflags). When the rig drifts — the unit goes back to `--user`,
binaries move, auth changes, the staging path changes — **edit the script, then
run it.** A stale script that silently deploys to the wrong path is the failure
mode this skill exists to prevent (it has happened: the script once targeted a
dead `--user` unit + `~/.local/bin`). If you change the contract, update the
header comment in `scripts/deploy-rig.sh` too.

## Run

```
# Full deploy (host build + composer + go), preflight-gated, self-verifying:
scripts/deploy-rig.sh

# Scope it down — env flags, all optional:
SKIP_COMPOSER=1   scripts/deploy-rig.sh    # Go/UI changed, C++ did not
SKIP_GO=1         scripts/deploy-rig.sh    # only composer/ C++ changed
SKIP_HOST_BUILD=1 scripts/deploy-rig.sh    # trust the host already compiles
RIG=user@host     scripts/deploy-rig.sh    # non-default ssh target
```

The orchestrator **stops hard, before changing anything, if the rig is off**:
preflight's ssh uses `BatchMode` + a 10s `ConnectTimeout`, so an unreachable or
auth-stalled rig fails fast and aborts (it never proceeds to stop the service).
After install it **hard-gates on `systemctl is-active`** — if the service does
not come back up it dumps the last 50 journal lines and exits non-zero, so a
dead service can never be reported as a green deploy. Don't paper over either
abort; fix the connectivity / startup failure (or amend the script if the
contract changed) and re-run.

Then verify a live stream (lazy-encoder-on-reader: the path 404s until a client
connects, so use a client that holds the connection — ffprobe/ffplay do):

```
STREAM=lyra composer/scripts/verify-from-dev.sh   # lyra/solo are h264; stream is h265
```

### Preflight first, always

`scripts/deploy-preflight.sh` runs as step 0 of the orchestrator, but run it
standalone whenever you just want to know the state of the rig. It does double
duty — the version query fails loudly if the box is unreachable, so there is no
separate "is the rig up" probe. If it prints `decision: UP-TO-DATE`, **stop,
the deploy is a no-op.** It also warns when the installed Go binary reports
`dev` — a prior deploy skipped the version ldflags.

`decision` is **per component, by source tree** — not whole-repo version-string
equality. The preflight resolves each installed version (`-g<sha>`) to its build
commit and diffs that commit..HEAD over the component's paths: Go binary ←
`*.go go.mod go.sum ui/`, C++ helpers ← `composer/ proto/`. So a Go-only change
correctly reports `cpp: UP-TO-DATE` even though the composer version string lags
HEAD — there is no reason to recompile C++ that did not change. Reasons it
prints: `no-version` (dev/absent — needs the ldflags build), `src-changed`,
`worktree-dirty` (uncommitted change in that component's paths), or
`unknown-commit:<ref>` (installed from a commit not in local history).

This makes scoping converge: a `SKIP_COMPOSER=1` Go deploy leaves the next
preflight at `UP-TO-DATE` instead of nagging forever. You usually don't need to
pick the flags by hand — `decision` already tells you which component is stale
(`go:src-changed` alone → `SKIP_COMPOSER=1`; `cpp:src-changed` alone →
`SKIP_GO=1`; both → full). A `fix(composer):` *commit* may be pure Go
(`internal/streams/...`); the per-path diff scopes by files, not the subject.

## Rig contract

- Go supervisor: `/usr/bin/videonode` (system unit `ExecStart=/usr/bin/videonode -c /etc/videonode/config.toml`, `User=videonode`, `WorkingDirectory=/etc/videonode`)
- Native helpers: `/usr/bin/videonode-{source,sink,composer}` (root-owned)
- Config: `/etc/videonode/{config.toml,streams.toml}`
- Unit: `videonode.service` — a systemd **system** unit. Manage with `sudo systemctl …`, never `--user` (`systemctl --user is-active videonode.service` returns `inactive`).
- API on `:8090`, RTSP on `:8554`. Stream paths come from `streams.toml` (currently `lyra`, `solo`, `stream`).
- Version queries: Go → `videonode version`; C++ → `videonode-composer --version`. The three native helpers are built+stamped together, so the composer's version stands in for all three.

## Gotchas

- **`Text file busy` on cp** — the service is still holding the binary mapped. The orchestrator stops the service before any `/usr/bin` cp; if you install by hand, `sudo systemctl stop videonode.service` first.
- **UI is `go:embed`-bundled behind the `ui_embed` tag.** Without `-tags ui_embed`, `ui/embed_fallback.go` ships and `ui.Handler()` redirects every UI route to `/docs` — the daemon is healthy and the API works, but `/streams` etc. serve no React app. `build-go-arm64.sh` always sets the tag and runs `pnpm build` first (the embed needs `ui/dist/`). Verify: `curl http://localhost:8090/streams | grep '<title>'` → `<title>VideoNode</title>`, not a redirect blob.
- **Version ldflags are mandatory.** A bare `go build` leaves `videonode version` as `dev (unknown)`, which blinds the preflight. `build-go-arm64.sh` injects `-X '…/internal/version.Version=$(git describe --tags --always --dirty)'`; never bypass it.
- **`/tmp/smoke-vn/` is a stale smoke artifact.** If a supervisor is running from `/tmp/smoke-vn/videonode`, `pkill -TERM -f /tmp/smoke-vn/videonode`, wait, `rm -rf /tmp/smoke-vn`. Never relaunch from there.
- **HDMI source needs signal lock.** `verify-from-dev.sh` against an HDMI stream FAILs on no signal — a source issue, not a deploy issue. The composer canvas stream (placeholder-renders when sources are absent) is the reliable verification target.
- **Auth backend drift.** The canonical service uses `[auth] username/password` from `/etc/videonode/config.toml` only when `[auth] type = "basic"` is set. With it unset it falls back to `linux`/PAM, which checks the system login (`REDACTED-CREDS`), not the documented `REDACTED-CREDS`. A wrong-cred `curl` returns a generic 401 with no hint which backend rejected.
- **Don't cross-build arm64 in local Docker via qemu** — `arm64v8/debian` + qemu-user-static on x86_64 is 10–30× slower; a full build is 30+ min. Build on the rig (CI's native arm64 runner is the only fast container path, not usable interactively).

## IPC-only check (faster, no canonical-service touch)

To validate just REST → pipelinectl → native helpers and per-binary RSS/CPU
without deploying, run `composer/scripts/smoke.sh --target rig` after a rig
build. Smoke spins its own ephemeral instance on `:8190`/`:8654` with private
sockets — it does not require or interact with the canonical service. ≤60s.

## Reporting

Re-running `scripts/deploy-preflight.sh` after the deploy IS the confirmation:
`service=active` plus `decision: UP-TO-DATE` proves the new code is live
(supersedes the old `ps -o etime` vs binary-mtime dance). Don't assert raw
version-string equality — for a scoped deploy the untouched component's string
legitimately lags HEAD; `decision: UP-TO-DATE` already accounts for that. Then
ffprobe a stream.

End the report with one verdict line:

- `verdict: PASS` — host build, sync, rig build, install, restart all succeeded; post-deploy preflight shows `service=active` and `decision: UP-TO-DATE`; and at least one RTSP stream ffprobes at the expected codec/resolution/fps.
- `verdict: FAIL (<reason>)` — reason ∈ `host build`, `sync`, `rig build`, `go build`, `install`, `service start`, `preflight stale` (post-deploy `decision:` still `DEPLOY-NEEDED`), `ffprobe`, `crash`.
- `verdict: DEPLOYED-NO-VERIFY` — deploy + restart succeeded and the caller explicitly skipped the ffprobe step.

On FAIL, surface:
- last ~50 lines of `sudo journalctl -u videonode.service` on the rig (system journal; `--user` is empty).
- coredump if one was produced:
  ```
  ssh orangepi 'sudo coredumpctl info --no-pager | head -100; echo ---; \
    LAST=$(sudo coredumpctl list --no-legend 2>/dev/null | tail -1 | awk "{print \$5}"); \
    [ -n "$LAST" ] && sudo coredumpctl info --no-pager "$LAST" 2>/dev/null'
  ```
- if post-deploy preflight is still `DEPLOY-NEEDED` for a component you just installed, the cp landed but the restart did not pick it up (or the component was skipped) — re-check the unit and `ps` for a lingering old PID.

## When NOT to use

- Pure host-side composer changes covered by `composer/tests/` — `ctest --test-dir composer/build/dev`.
- Frontend-only changes unrelated to the composer tree.
- IPC-contract validation only — `composer/scripts/smoke.sh --target rig` is faster.
- "Does it still compile cleanly with sanitizers" — host `cmake --build --preset dev` alone.
- Rig offline, or the deploy target stream has no signal — deploy succeeds but `verify-from-dev.sh` FAILs on the source, not your change. Verify against the composer canvas stream instead.

## Prerequisites

- Host: `cmake` (≥3.20), `ninja`, a C++ compiler + composer deps (`composer/README.md`), `rsync`, `ssh` to `orangepi5-ultra.lan`, and a Go toolchain (for the Go cross-build path).
- Rig: `videonode.service` installed + enabled as a **system** unit; `/etc/videonode/{config.toml,streams.toml}` populated; `sudo` (the `orangepi` login has it); `coredumpctl`; HDMI-IN and/or the Hollyland Lyra UVC camera connected.
