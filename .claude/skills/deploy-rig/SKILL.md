---
name: deploy-rig
description: Build composer native binaries on host, deploy to the RK3588 SBC (orangepi5-ultra.lan), build them rig-native, install into ~/.local/bin, and restart the canonical videonode systemd user service. Use after non-trivial composer/ or Go-side changes that need real-hardware verification.
---

# composer deploy-rig

End-to-end real-hardware deploy + restart for the videonode rig. Builds locally (host sanity), rsyncs `composer/` to the rig, builds the native binaries rig-native, installs them into `~/.local/bin/`, optionally cross-builds the Go `videonode` supervisor for arm64 and installs that too, then restarts the `videonode.service` user unit so the running pipeline picks up the new bits.

The composer is daemon-driven: `videonode-source`, `videonode-sink`, and `videonode-composer` run as children of the Go `videonode` supervisor, coordinated over the pipelinectl IPC socket. The canonical install layout on the rig is:

- Go supervisor: `/home/orangepi/.local/bin/videonode`
- Native helpers: `/home/orangepi/.local/bin/videonode-{source,sink,composer}`
- Config: `/home/orangepi/.config/videonode/{config.toml,streams.toml}`
- Systemd user unit: `videonode.service`

The supervisor exposes the API on `:8090` and serves RTSP on `:8554`. Per-stream paths are defined in `streams.toml` (e.g. `rtsp://orangepi5-ultra.lan:8554/r2`, `.../gpucanvas`).

**Do not run anything out of `/tmp/smoke-vn/`**. That directory is a stale smoke-test artifact; if it exists, delete it and let smoke recreate its own scratch dir on the next run.

## Run

From the repo root:

```
# 1. Host sanity build (sanitizers + Ninja).
cd composer && cmake --preset dev && cmake --build --preset dev && cd ..

# 2. Sync composer/ to the rig and build rig-native binaries.
composer/scripts/sync-to-rig.sh
RIG=orangepi composer/scripts/build-on-rig.sh

# 3. (Optional) cross-build Go supervisor and install over the canonical bin.
#
# The UI is embedded into the binary via `go:embed ui/dist`. If you
# touched anything under `ui/src/`, you MUST `pnpm build` first or
# the binary ships the stale pre-build assets (or fails to compile
# when ui/dist doesn't exist yet). Safe to run unconditionally:
cd ui && pnpm install --frozen-lockfile && pnpm build && cd ..
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o /tmp/videonode-arm64 .
# Direct scp over the live canonical binary fails with "Failure" when
# the service is holding the text segment mapped. Stage to /tmp first,
# then cp inside step 4 (which has the service stopped):
scp /tmp/videonode-arm64 orangepi:/tmp/videonode-arm64.staging

# 4. Stop the service (free Text file busy locks), install the native bins
#    AND the staged Go supervisor (if step 3 ran), then start the service.
ssh orangepi '
  systemctl --user stop videonode.service
  [ -f /tmp/videonode-arm64.staging ] && cp -f /tmp/videonode-arm64.staging /home/orangepi/.local/bin/videonode
  cp -f /home/orangepi/composer/build/src/bin/videonode-source   /home/orangepi/.local/bin/
  cp -f /home/orangepi/composer/build/src/bin/videonode-sink     /home/orangepi/.local/bin/
  cp -f /home/orangepi/composer/build/src/bin/videonode-composer /home/orangepi/.local/bin/
  chmod +x /home/orangepi/.local/bin/videonode*
  systemctl --user start videonode.service
'

# 5. Verify.
ssh orangepi 'systemctl --user is-active videonode.service'
composer/scripts/verify-from-dev.sh
```

Tunables (env vars, all optional):

- `RIG` — ssh target. Default `orangepi` (from `~/.ssh/config`); a full host like `orangepi@orangepi5-ultra.lan` works too.
- `STREAM` (for `verify-from-dev.sh`) — RTSP stream path. Default `composer`; for the canonical service use `gpucanvas` or whatever path your `streams.toml` exposes.
- `EXPECT_W` / `EXPECT_H` — expected canvas dimensions. Defaults `1920` / `1080`.

If you only need to validate the IPC contract (REST -> pipelinectl -> native helpers) and per-binary RSS/CPU without touching the canonical service, run `composer/scripts/smoke.sh --target rig` after step 2. Smoke spins up its own ephemeral `videonode` instance on ports `:8190` / `:8654` with private sockets — it does not require, and does not interact with, the canonical service. Total smoke wall time ≤ 60s.

If a deploy or restart fails or a binary crashes, capture coredump info before declaring FAIL:

```
ssh orangepi 'coredumpctl info --no-pager | head -100; \
  echo ---; \
  LAST=$(coredumpctl list --no-legend 2>/dev/null | tail -1 | awk "{print \$5}"); \
  [ -n "$LAST" ] && coredumpctl info --no-pager "$LAST" 2>/dev/null'
```

## What it does

1. **Host build** — `cmake --preset dev && cmake --build --preset dev` from `composer/`. The `dev` preset enables sanitizers + Ninja. Host build is a sanity check that the code compiles cleanly with the strict preset before any bytes hit the rig. The binaries that actually run are the rig-native ones from step 2.
2. **Sync + rig build** — `sync-to-rig.sh` rsyncs `composer/` to `orangepi:/home/orangepi/composer/` with `--delete`, excluding `build/` and `.cache/`. `build-on-rig.sh` then ssh's in, runs cmake -> ninja, and produces `videonode-{source,sink,composer}` under `/home/orangepi/composer/build/src/bin/`. **Note: those are not yet on the canonical install path.**
3. **Go cross-build (optional)** — if the Go side has changed too, `GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build` produces an arm64 binary and `scp` installs it over `/home/orangepi/.local/bin/videonode`. Skip when only `composer/` has changed.
4. **Install + restart** — stop the service (so the running process releases its text segment; otherwise `cp` fails with `Text file busy`), `cp -f` the freshly-built native bins from the composer build dir into `~/.local/bin/`, then start the service. The supervisor spawns the new native helpers as children.
5. **Verify** — check `systemctl --user is-active`, then `verify-from-dev.sh` (or a manual `ffprobe`) against the chosen RTSP path.

## Gotchas

- **Canonical install path matters.** The systemd unit runs `/home/orangepi/.local/bin/videonode`, which by default looks up its native helpers under `~/.local/bin/`. Native bins sitting only at `/home/orangepi/composer/build/src/bin/` will be ignored by the canonical service — you must `cp` them across (step 4).
- **`Text file busy`** on `cp` means the service is still running and holding the binary mapped. Always `systemctl --user stop videonode.service` before installing native helpers.
- **`/tmp/smoke-vn/`** is a stale smoke-test scratch dir. If you find a supervisor running from `/tmp/smoke-vn/videonode`, kill it (`pkill -TERM -f /tmp/smoke-vn/videonode`), wait, then `rm -rf /tmp/smoke-vn`. Do not relaunch from there.
- **Running processes hold old code.** A successful `cp` updates the binary on disk, but already-running processes keep their old text segment mapped. Confirm the new code is live by comparing `ps -o etime` for the supervisor + helpers against the binary mtime on the rig.
- **HDMI source unstable** — `verify-from-dev.sh` against the HDMI stream will fail if the input has no signal lock. That is a source issue, not a deploy issue; the composer canvas stream (which falls back to placeholder rendering when sources are absent) is the more reliable verification target.
- **UI is `go:embed`-bundled** — touching anything under `ui/src/` requires `cd ui && pnpm build` BEFORE `go build`, otherwise the binary ships the stale pre-build assets in `ui/dist/`. Safe to run the UI build unconditionally; it's ~5 seconds. The bare `go build` succeeds without it (existing `ui/dist/` from a prior build is fine), so the staleness is silent — you'll only notice by diffing the rendered HTML.
- **Auth on the canonical service is Linux PAM**, not the `[auth] username/password` fields in `config.toml`. The factory defaults to `linux` when `[auth] type` is unset, which hands credentials to `/sbin/unix_chkpwd` for the system user. On the rig that's `REDACTED-CREDS`. The `[auth] username/password` config fields are only consulted when `[auth] type = "basic"` is set explicitly. `curl -u <wrong>:<wrong>` returns 401 with no hint about which backend is rejecting.

## Prerequisites

- Host: `cmake` (>=3.20), `ninja`, a working C++ compiler, and the composer deps listed in `composer/README.md`.
- Host: `rsync`, `ssh`, network reachability to `orangepi5-ultra.lan`.
- Host: a Go toolchain — only needed if step 3 (Go cross-build) is in scope.
- Rig: `coredumpctl` available (Armbian default); HDMI-IN feeding `/dev/v4l/by-path/platform-fdee0000.hdmirx-controller-video-index0`.
- Rig: `~/.config/systemd/user/videonode.service` installed and enabled, with `~/.config/videonode/{config.toml,streams.toml}` populated.

## Reporting

End the report with one verdict line:

- `verdict: PASS` — host build, sync, rig build, install, and service-restart all succeeded; `systemctl --user is-active` returns `active`; and at least one RTSP stream ffprobes as `h264 1920x1080` at the expected frame rate.
- `verdict: FAIL (<short reason>)` — short reason in: `host build`, `sync`, `rig build`, `install`, `service start`, `ffprobe`, `crash`.
- `verdict: DEPLOYED-NO-VERIFY` — sync + rig build + install + restart all succeeded and the caller explicitly skipped the ffprobe step.

On FAIL also surface:
- The last ~50 lines of `journalctl --user -u videonode.service` on the rig.
- `coredumpctl` output if a core was produced (see command above).
- The mtimes of `~/.local/bin/videonode*` versus `ps -o etime` for the supervisor — if the running PID is older than the binary, the cp landed but the restart did not.

Manually verifying a live RTSP feed (canonical service):

```
ffplay -rtsp_transport tcp rtsp://orangepi5-ultra.lan:8554/gpucanvas
# or, scripted:
STREAM=gpucanvas composer/scripts/verify-from-dev.sh
```

## When NOT to use

- Pure host-side composer changes already covered by `composer/tests/` — run those directly via `ctest --test-dir composer/build/dev`.
- Frontend-only changes — unrelated to the composer tree.
- IPC-contract validation only — `composer/scripts/smoke.sh --target rig` is faster and does not touch the canonical service.
- Quick "does it still compile cleanly with sanitizers" check — host `cmake --build --preset dev` alone is enough.
- When the rig is offline or the HDMI-IN source is not connected — the deploy will succeed but `verify-from-dev.sh` against the HDMI stream will FAIL on lack of signal, not on anything you changed. Use the canvas stream for verification in that case.
