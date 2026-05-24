---
name: deploy-rig
description: Build composer native binaries inside an arm64 Docker container, install on the RK3588 SBC (orangepi5-ultra.lan) via the resulting .deb to /usr/bin, optionally cross-build the Go videonode supervisor and stage it to ~/.local/bin/videonode, then restart the canonical videonode systemd user service. Use after non-trivial composer/ or Go-side changes that need real-hardware verification.
---

# composer deploy-rig

End-to-end real-hardware deploy + restart for the videonode rig. **All native-binary compilation happens inside an arm64v8/debian:trixie Docker container** — building on the rig itself has been observed to destabilise the running hdmirx capture pipeline and is no longer the supported path. The Docker build produces a Debian package (`composer/build/deb-arm64/*.deb`); the deploy script `scp`s that .deb to the rig and `dpkg -i`s it, dropping `/usr/bin/videonode-{source,sink,composer}` plus a systemd-user environment override that points the Go daemon at those `/usr/bin` paths.

The composer is daemon-driven: `videonode-source`, `videonode-sink`, and `videonode-composer` run as children of the Go `videonode` supervisor, coordinated over the pipelinectl IPC socket. The canonical install layout on the rig after a successful deploy is:

- Go supervisor: `/home/orangepi/.local/bin/videonode`
- Native helpers (from the .deb): `/usr/bin/videonode-{source,sink,composer}`
- systemd-user drop-in pointing the daemon at the .deb paths: `~/.config/systemd/user/videonode.service.d/native-pipeline.conf`
- Config: `/home/orangepi/.config/videonode/{config.toml,streams.toml}`
- Systemd user unit: `videonode.service`

The supervisor exposes the API on `:8090` and serves RTSP on `:8554`. Per-stream paths are defined in `streams.toml` (e.g. `rtsp://orangepi5-ultra.lan:8554/r2`, `.../gpucanvas`).

**Do not run anything out of `/tmp/smoke-vn/`**. That directory is a stale smoke-test artifact; if it exists, delete it and let smoke recreate its own scratch dir on the next run.

## Run

From the repo root:

```
# 1. Host sanity build (sanitizers + Ninja).
cd composer && cmake --preset dev && cmake --build --preset dev && cd ..

# 2. Build the arm64 .deb inside a Docker container and install it on the rig.
#    This stops videonode.service, dpkg -i's the .deb (lands /usr/bin/videonode-*),
#    writes the systemd-user env override, and reloads systemd. It does NOT
#    start the service yet — step 4 does that after the optional Go install.
#
#    Pass SKIP_BUILD=1 to reuse the most recent composer/build/deb-arm64/*.deb
#    without rebuilding (faster iteration when only the Go side changed).
RIG=orangepi composer/scripts/build-deb-install-rig.sh

# 3. (Optional) cross-build Go supervisor and stage it for install.
#
# The UI is embedded into the binary via `go:embed ui/dist`, but ONLY
# when the `ui_embed` build tag is set. Without `-tags ui_embed` the
# fallback Handler() ships (just redirects every UI route to /docs);
# the daemon is healthy but `/streams` etc. serve no React app. The
# tag-gated embed wants `ui/dist/` to exist, so `pnpm build` is a
# hard prerequisite (~5s). Safe to run all three unconditionally:
cd ui && pnpm install --frozen-lockfile && pnpm build && cd ..
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -tags ui_embed -o /tmp/videonode-arm64 .
# Direct scp over the live canonical binary fails with "Failure" when
# the service is holding the text segment mapped. Stage to /tmp first,
# then cp inside step 4 (which has the service stopped):
scp /tmp/videonode-arm64 orangepi:/tmp/videonode-arm64.staging

# 4. Install the staged Go supervisor (if step 3 ran) and start the service.
#    The native helpers are already installed by step 2.
ssh orangepi '
  [ -f /tmp/videonode-arm64.staging ] && cp -f /tmp/videonode-arm64.staging /home/orangepi/.local/bin/videonode
  chmod +x /home/orangepi/.local/bin/videonode
  systemctl --user start videonode.service
'

# 5. Verify.
ssh orangepi 'systemctl --user is-active videonode.service'
composer/scripts/verify-from-dev.sh
```

Tunables (env vars, all optional):

- `RIG` — ssh target. Default `orangepi` (from `~/.ssh/config`); a full host like `orangepi@orangepi5-ultra.lan` works too.
- `SUDO` — elevation tool used on the rig for `dpkg -i`. Default `sudo`.
- `SKIP_BUILD` — set to `1` to reuse the most recent `composer/build/deb-arm64/*.deb` instead of rebuilding in Docker. Useful when iterating on the Go side only.
- `ENGINE` / `IMAGE` — passed through to `build-deb-arm64-docker.sh`. Default container engine is autodetected (docker, then podman); default image is `arm64v8/debian:trixie`.
- `STREAM` (for `verify-from-dev.sh`) — RTSP stream path. Default `composer`; for the canonical service use `gpucanvas` or whatever path your `streams.toml` exposes.
- `EXPECT_W` / `EXPECT_H` — expected canvas dimensions. Defaults `1920` / `1080`.

If you only need to validate the IPC contract (REST -> pipelinectl -> native helpers) and per-binary RSS/CPU without touching the canonical service, run `composer/scripts/smoke.sh --target rig`. Smoke spins up its own ephemeral `videonode` instance on ports `:8190` / `:8654` with private sockets — it does not require, and does not interact with, the canonical service. Total smoke wall time ≤ 60s. (Smoke continues to use `scripts/build-on-rig.sh` for its own scratch builds; the canonical-deploy .deb at `/usr/bin/` is separate.)

If a deploy or restart fails or a binary crashes, capture coredump info before declaring FAIL:

```
ssh orangepi 'coredumpctl info --no-pager | head -100; \
  echo ---; \
  LAST=$(coredumpctl list --no-legend 2>/dev/null | tail -1 | awk "{print \$5}"); \
  [ -n "$LAST" ] && coredumpctl info --no-pager "$LAST" 2>/dev/null'
```

## What it does

1. **Host build** — `cmake --preset dev && cmake --build --preset dev` from `composer/`. The `dev` preset enables sanitizers + Ninja. Host build is a sanity check that the code compiles cleanly with the strict preset before any bytes hit the container. The binaries that actually run are the arm64 ones produced by step 2.
2. **Docker .deb build + install** — `build-deb-install-rig.sh` runs `build-deb-arm64-docker.sh` to compile inside `arm64v8/debian:trixie` (qemu-user-static emulates aarch64 on x86_64), produces `composer/build/deb-arm64/*.deb`, `scp`s it to the rig, then on the rig: stops the service, `dpkg -i`s the .deb, writes the systemd-user `NATIVE_PIPELINE_*` environment override, and reloads systemd. The service is left **stopped** so step 4 (which may install a fresh Go supervisor) doesn't have to stop it again. Native helpers land at `/usr/bin/videonode-{source,sink,composer}`.
3. **Go cross-build (optional)** — if the Go side has changed too, `GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -tags ui_embed` produces an arm64 binary and `scp` stages it to `/tmp/videonode-arm64.staging` on the rig. Skip when only `composer/` has changed.
4. **Install + restart** — `cp -f` the staged Go supervisor into `~/.local/bin/videonode` if present, then `systemctl --user start videonode.service`. The supervisor reads `NATIVE_PIPELINE_*` from its drop-in and spawns the `/usr/bin/videonode-*` helpers as children.
5. **Verify** — check `systemctl --user is-active`, then `verify-from-dev.sh` (or a manual `ffprobe`) against the chosen RTSP path.

## Gotchas

- **The rig now runs the .deb, not the worktree build.** Earlier deploys cp'd from `/home/orangepi/composer/build/src/bin/` into `~/.local/bin/`. After this change the canonical service runs `/usr/bin/videonode-*` from the .deb, and the systemd drop-in `Environment=NATIVE_PIPELINE_*` is what makes the daemon find them. If the drop-in is missing, the daemon falls back to its `~/.local/bin/videonode-*` defaults, which is likely a stale install.
- **`Text file busy`** on the .deb install means the service is still running and holding the binary mapped. `build-deb-install-rig.sh` stops the service before `dpkg -i`; do not skip that.
- **`/tmp/smoke-vn/`** is a stale smoke-test scratch dir. If you find a supervisor running from `/tmp/smoke-vn/videonode`, kill it (`pkill -TERM -f /tmp/smoke-vn/videonode`), wait, then `rm -rf /tmp/smoke-vn`. Do not relaunch from there.
- **qemu-user-static binfmt** must be registered on the host for arm64 emulation to work in Docker. `build-deb-arm64-docker.sh` tries to register it via `multiarch/qemu-user-static --reset -p yes`, but if `/proc/sys/fs/binfmt_misc/qemu-aarch64` already exists with `enabled` set, you're good without sudo.
- **Running processes hold old code.** A successful `dpkg -i` updates the binary on disk, but already-running processes keep their old text segment mapped. The deploy script stops the service before installing, so the new code is live as soon as step 4 starts it.
- **HDMI source unstable** — `verify-from-dev.sh` against the HDMI stream will fail if the input has no signal lock. That is a source issue, not a deploy issue; the composer canvas stream (which falls back to placeholder rendering when sources are absent) is the more reliable verification target.
- **UI is `go:embed`-bundled behind the `ui_embed` build tag** — `go build` WITHOUT `-tags ui_embed` compiles `ui/embed_fallback.go` instead of `ui/embed.go`, and `ui.Handler()` redirects every UI route to `/docs`. The daemon is healthy and the API works, but `/streams` etc. serve no React app — it looks like the UI was never built. Tag-gated `go build -tags ui_embed` requires `ui/dist/` to exist, so `cd ui && pnpm build` is a hard prerequisite. The bundled binary jumps from ~36 MB to ~40 MB when the embed actually lands; check `ls -la` on the staging file before deploying. Diff the served HTML via `curl http://localhost:8090/streams | grep '<title>'` — should be `<title>VideoNode</title>`, not a `Found` redirect blob.
- **Auth on the canonical service** uses the `[auth] username/password` fields in `config.toml`, courtesy of `[auth] type = "basic"` (added 2026-05; pre-2026-05 the unset `type` defaulted to `linux`/PAM, which silently checked the system user `REDACTED-CREDS` instead). Today's canonical creds: `REDACTED-CREDS`. If a fresh install or a reverted config drops `type = "basic"`, the factory falls back to `linux` and `curl -u REDACTED-CREDS` will return 401 — re-add `type = "basic"` to `[auth]` and restart. `curl -u <wrong>:<wrong>` returns a generic 401 with no hint about which backend rejected.

## Prerequisites

- Host: `docker` or `podman` with `qemu-user-static` binfmt registered (check `/proc/sys/fs/binfmt_misc/qemu-aarch64`).
- Host: `cmake` (>=3.30), `ninja`, a working C++ compiler — only needed for step 1's host-side sanity build.
- Host: `ssh`, `scp`, network reachability to `orangepi5-ultra.lan`.
- Host: a Go toolchain — only needed if step 3 (Go cross-build) is in scope.
- Rig: passwordless `sudo` for `dpkg -i` (or override via `SUDO=`); HDMI-IN feeding `/dev/v4l/by-path/platform-fdee0000.hdmirx-controller-video-index0`.
- Rig: `~/.config/systemd/user/videonode.service` installed and enabled, with `~/.config/videonode/{config.toml,streams.toml}` populated.

## Reporting

End the report with one verdict line:

- `verdict: PASS` — host build, .deb build, .deb install, optional Go install, and service-restart all succeeded; `systemctl --user is-active` returns `active`; and at least one RTSP stream ffprobes as `h264 1920x1080` at the expected frame rate.
- `verdict: FAIL (<short reason>)` — short reason in: `host build`, `docker build`, `scp`, `dpkg`, `service start`, `ffprobe`, `crash`.
- `verdict: DEPLOYED-NO-VERIFY` — .deb installed + service started, caller explicitly skipped the ffprobe step.

On FAIL also surface:
- The last ~50 lines of `journalctl --user -u videonode.service` on the rig.
- `coredumpctl` output if a core was produced (see command above).
- The mtimes of `/usr/bin/videonode*` and `~/.local/bin/videonode` versus `ps -o etime` for the supervisor — if the running PID is older than the binary, the install landed but the restart did not.

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
