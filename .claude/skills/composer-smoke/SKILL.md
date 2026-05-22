---
name: composer-smoke
description: Fast end-to-end smoke test for the composer pipeline (videonode-source → SCM_RIGHTS → videonode-sink / videonode-composer). Exercises HDMI-IN capture, MJPEG (if a UVC device is present), consumer connect/disconnect churn, and reports per-binary RSS + %CPU. Use after non-trivial composer/ changes, before /commit on anything in src/ipc, src/source, src/render, src/capture, or whenever you want to confirm the rig pipeline still flows frames. Faster than /deploy-rig — runs all binaries to completion in ≤60s per scenario.
---

# composer-smoke

Runs `composer/scripts/smoke.sh` against the rig. Each scenario produces one of PASS / FAIL / SKIP and reports peak RSS + average %CPU for every binary it spawned. Exits 0 if no FAILs (SKIPs are OK), 1 otherwise.

The host has no HDMI source so no host-side scenarios run today. Add a future `H*` scenario only if it actually exercises a host-runnable path.

## Run

From the repo root:

```bash
# Default: try both host and rig with 4-second captures per scenario
composer/scripts/smoke.sh

# Rig only (preferred when iterating on rig-only paths like HDMI-IN + RGA)
composer/scripts/smoke.sh --target rig --duration 4

# Host only (fast iteration on lavfi compose path)
composer/scripts/smoke.sh --target host

# Subset
composer/scripts/smoke.sh --target rig --scenarios R1,R3,R4

# Keep /tmp/smoke-composer{,-rig} for inspection on failure
composer/scripts/smoke.sh --keep-artifacts
```

Stream output verbatim. The last block is a summary; surface it to the user.

## Tunables (env)

- `RIG` — ssh target. Default `orangepi` (alias from `~/.ssh/config`).
- `RIG_SSH` — full ssh command. Override when the default agent is unhappy:
  `RIG_SSH="ssh -i ~/.ssh/myrig_key" composer/scripts/smoke.sh --target rig`.
- `RIG_BUILD` — rig build dir. Default `/home/orangepi/composer/build`.
- `HOST_BUILD` — local build dir. Default `/tmp/composer-build` (used when
  `composer/build` is root-owned from an earlier container run; otherwise
  prefer `composer/build/dev`).
- `ARTIFACTS_DIR` — host-side artifact directory. Default `/tmp/smoke-composer`.

## Scenarios

R-prefix scenarios exercise the raw C++ binaries directly (no Go
daemon). I-prefix scenarios spawn a smoke-owned `videonode` daemon (a
fresh build of the working tree at `/tmp/smoke-vn/videonode`, custom
config on port `:8190`, RTSP `:8654`) and drive it via REST to validate
the daemon → pipelinectl → composer IPC contract end-to-end.

| # | Name | Target | What it asserts |
|---|---|---|---|
| R1 | hdmiin-source-sink | rig | `videonode-source` on `/dev/video0` → SCM → `videonode-sink` → Y4M; source's `real+placeholder` rate ≥30fps (placeholder) or ≥60fps (HDMI locked) |
| R4 | consumer-reconnect | rig | 5× sink connect/disconnect; source survives, no socket/dma-buf fd leak in `/proc/$pid/fd` after settle |
| R6 | mjpeg-uvc | rig | Auto-detects UVC MJPEG device, SKIP if absent |
| I1 | ipc-canvas-perspective | rig | Smoke spawns own `videonode` on `:8190`. POST source + canvas, engage. Asserts: composer identifies as `kind=composer`, initial set_canvas/set_source/set_layout/set_source_state push lands. PATCH perspective. Asserts: composer PID unchanged (no restart), daemon logs `perspective updated via IPC; canvas not restarted`, RTSP delivers 60 BGRA frames @ ≥70% target fps, ffprobe sees `codec=h264`. |
| I2 | ipc-resource-usage | rig | Reuses I1's running smoke daemon; samples RSS+%CPU for `videonode-source` + `videonode-composer` over 10s. Fails if either dies mid-window or if source >200 MB / composer >400 MB. |

Every PASS / FAIL line includes per-binary `name=NMb/X.Y%` resource readout when applicable, so a green run still shows you what each component cost.

I1 cross-compiles `videonode` for arm64 (`GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build`) and rsyncs to `/tmp/smoke-vn/videonode` on the rig before each run — the working tree's binary at a smoke-only path.

## Pre-flight on the rig

- HDMI signal stability is probed once (3 successive `VIDIOC_QUERY_DV_TIMINGS` queries across 600ms). `R_HDMI_LOCKED=1` tightens R1's threshold to 60fps; without a stable lock R1 falls back to 30fps target (placeholder mode).
- C++ binaries must exist at `$RIG_BUILD/src/bin/videonode-{source,sink,composer}` (run `composer/scripts/sync-to-rig.sh && composer/scripts/build-on-rig.sh` if missing).

## What the smoke is NOT

- Not a replacement for `/deploy-rig` — that pushes to RTSP and validates the encoder chain. The smoke pipes BGRA/Y4M to a file or `ffprobe` and checks frame flow / RSS / CPU only.
- Not a video-quality check — golden-frame SSIM/PSNR would catch regressions like the `ee82f2f` NV12 UV-offset bug; this smoke would not. Adding `ssim/psnr` filters is the next layer up.
- Not a daemon test — R7 is a one-shot `/api/health` ping. The Go-daemon REST surface lives in `/smoke-test`.

## Reporting

End the user-visible report with one line:

- `result: PASS — N PASS / 0 FAIL / M SKIP` if everything passes.
- `result: FAIL — N PASS / K FAIL / M SKIP (first FAIL: …)` if anything failed; include the first-FAIL detail line verbatim. Mention that `/tmp/smoke-composer` and `/tmp/smoke-composer-rig` retain logs when artifacts are kept.

## When NOT to use

- Pure host-only changes covered by `composer/tests/` — run `ctest --preset dev` directly.
- Frontend / Go-daemon changes — use the existing `/smoke-test` skill or `cd ui && pnpm typecheck`.
- When the rig is offline — the smoke will report `rig-ssh SKIP …`; you'll see only host coverage. Don't manually try to bring up the rig; that's outside scope.
