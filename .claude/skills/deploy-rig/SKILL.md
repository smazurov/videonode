---
name: deploy-rig
description: Build composer binaries on host, deploy to RK3588 SBC (orangepi5-ultra.lan), run the live-spike RTSP pipeline for N seconds, capture stderr + coredump info on crash. Use after non-trivial composer/ changes that need real-hardware verification.
---

# composer deploy-rig

End-to-end real-hardware check for the `composer/` tree. Builds locally, rsyncs to the rig, runs the live HDMI-IN + Lyra -> GLES compose -> `h264_rkmpp` -> RTSP pipeline, and surfaces any coredump if the spike crashed.

## Run

From the repo root (override `SECONDS_RUN` to change the run length; default `10`):

```
cd composer && cmake --preset dev && cmake --build --preset dev && cd .. \
  && composer/scripts/sync-to-rig.sh \
  && SECONDS_RUN=10 composer/scripts/run-spike-rig.sh
```

Stream the output verbatim. The skill's last line is the verdict (see Reporting). If `run-spike-rig.sh` exits non-zero or the SSH session reports a crash signal, capture coredump info before declaring FAIL:

```
ssh orangepi@orangepi5-ultra.lan 'coredumpctl info --no-pager | head -100; \
  echo ---; \
  LAST=$(coredumpctl list --no-legend 2>/dev/null | tail -1 | awk "{print \$5}"); \
  [ -n "$LAST" ] && coredumpctl info --no-pager "$LAST" 2>/dev/null'
```

Tunables (env vars, all optional):

- `SECONDS_RUN` — pipeline run length in seconds. Default `10`. `0` means run until killed.
- `STREAM_NAME` — RTSP path component. Default `spike` (URL: `rtsp://orangepi5-ultra.lan:8554/spike`).
- `CANVAS_W` / `CANVAS_H` / `CANVAS_FPS` — composer canvas. Defaults `1920` / `1080` / `30`.
- `RIG` — ssh target. Default `orangepi@orangepi5-ultra.lan`.

## What it does

1. **Build locally** — `cmake --preset dev && cmake --build --preset dev` from `composer/`. Phase B's `dev` preset enables sanitizers + Ninja. The host build is a sanity check that the code compiles cleanly with the strict preset before any bytes hit the rig; the binaries that actually run are the rig-native ones produced in step 3.
2. **Sync** — `composer/scripts/sync-to-rig.sh` rsyncs `composer/` to `orangepi@orangepi5-ultra.lan:/home/orangepi/composer-spike/` with `--delete`, excluding `build/` and `.cache/`. No build artifacts cross the wire.
3. **Build on rig** — the rig-side build happens via `scripts/build-on-rig.sh` if `composer-spike` isn't already present in `/home/orangepi/composer-spike/build/`. `run-spike-rig.sh` aborts with a clear error if the binary is missing, so run `composer/scripts/build-on-rig.sh` once after a fresh clone.
4. **Run** — `composer/scripts/run-spike-rig.sh` ssh's into the rig, starts `mediamtx` if not running, then pipes `composer-spike` (HDMI-IN 4K NV12 + Lyra 1080p MJPEG, GLES-composed to BGRA on stdout) into `ffmpeg -c:v h264_rkmpp` -> RTSP on `127.0.0.1:8554/${STREAM_NAME}`. Runs for `SECONDS_RUN` seconds, then `composer-spike` exits and the trap kills `mediamtx`.
5. **Capture crash info on failure** — if the SSH session exits non-zero, ssh back in and dump the most recent `coredumpctl` entry (see the command in Run). Surface stderr tail + coredump info to the user.

## Prerequisites

- Host: `cmake` (>=3.20), `ninja`, a working C++ compiler, and the composer deps listed in `composer/README.md`.
- Host: `rsync`, `ssh`, network reachability to `orangepi5-ultra.lan`.
- Rig: composer-spike built once via `composer/scripts/build-on-rig.sh`; `mediamtx` installed via `composer/scripts/install-mediamtx.sh`; `coredumpctl` available (Armbian default).
- Rig: HDMI-IN feeding `/dev/video*` and the Lyra MJPEG source plugged in — otherwise `composer-spike` will exit early with a source error, which the skill will report as FAIL with a clear stderr message.

## Reporting

End the report with one verdict line:

- `verdict: PASS` — host build succeeded, sync succeeded, `run-spike-rig.sh` exited 0 after `SECONDS_RUN` seconds.
- `verdict: FAIL (<short reason>)` — host build failed, sync failed, or the rig-side pipeline exited non-zero / crashed. Reasons: `host build`, `sync`, `rig binary missing`, `pipeline crash`, `pipeline non-zero exit`, `mediamtx missing`.

On FAIL also surface:
- The last ~50 lines of the SSH session's stderr (composer-spike + ffmpeg + mediamtx logs).
- `coredumpctl` output if a core was produced (see command in Run).
- The mediamtx log path on the rig: `/tmp/mediamtx.log`.

Verifying the RTSP feed manually (not part of the skill's automated verdict, since the pipeline tears down at `SECONDS_RUN`):

```
ffplay -rtsp_transport tcp rtsp://orangepi5-ultra.lan:8554/spike
```

## When NOT to use

- Pure host-side composer changes already covered by `composer/tests/` — run those directly.
- Frontend-only changes — unrelated to the composer tree.
- Quick iteration where you don't need to confirm the `h264_rkmpp` encoder still accepts the BGRA stream — sync + rig build is ~15-30s on top of the run length.
- When the rig is offline or HDMI-IN/Lyra sources aren't connected — the skill will FAIL on the source-open step, not on anything you changed.
