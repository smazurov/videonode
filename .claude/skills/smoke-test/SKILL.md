---
name: smoke-test
description: Full end-to-end smoke test for videonode. Builds the binary, validates the hardware encoder stack, spawns an isolated server on ephemeral ports with a test_mode stream, exercises the REST API, and ffprobes the live RTSP + SRT outputs. Use after non-trivial backend changes, before /commit on API/streaming/encoder/ffmpeg changes, and after deploying to the SBC.
---

# videonode smoke test

## Run

From the repo root:

```
go test -v -count=1 -timeout=180s -tags=smoke ./test/smoke/...
```

That's it. Stream the output to the user verbatim. The last line of `go test` output is either `ok ...` (PASS) or `FAIL ...` (FAIL).

## What it does

The Go suite under `test/smoke/` does all the orchestration:

1. **Build** — `go build -o <tmpdir>/videonode .` from the repo root.
2. **Encoder validation** — runs `videonode validate-encoders -q` with the temp dir as cwd so it writes to the isolated `streams.toml`.
3. **Platform detection** — inspects `/proc/device-tree/compatible`, `/proc/driver/nvidia`, `lspci`, and `/dev/dri` to determine the expected encoder family (`mpp` / `nvenc` / `vaapi` / `software`).
4. **Server spawn** — starts `videonode` on ephemeral HTTP/RTSP/SRT ports with `AUTH_TYPE=basic AUTH_USERNAME=smoke AUTH_PASSWORD=smoke` and `STREAMS_CONFIG_FILE` pointed at the temp `streams.toml` (which contains a `test_mode = true` bootstrap stream `smoke-pipeline`).
5. **API surface tests** — `/api/health`, `/api/metrics` (with and without auth), `GET /api/streams`, `GET /api/encoders`, `GET /api/devices`, full CRUD cycle on a temporary stream.
6. **Encoder family assertion** — parses `streams.toml`'s `[validation.h264].working` / `[validation.h265].working` and fails if the expected family is absent.
7. **Pipeline E2E** — tails `/api/events` (SSE) until `stream-state-changed` with `action=running` arrives for `smoke-pipeline`, in parallel polls `GET /api/streams/smoke-pipeline` for `enabled=true`, then ffprobes both the RTSP and SRT outputs and asserts `codec_name=h264`.
8. **Teardown** — sends SIGTERM to the server's process group (kills FFmpeg children too), waits up to 5s, then SIGKILL. Removes the temp dir on PASS, keeps it on FAIL.

## Prerequisites

- `go` (1.25+, per `go.mod`)
- `ffmpeg` and `ffprobe` in `PATH` — videonode needs FFmpeg to run the `test_mode` stream, and the suite uses `ffprobe` to verify the pipeline.

## Reporting

Surface the final `PASS`/`FAIL` line from `go test`. On FAIL:
- Surface the `smoke run dir kept at: /tmp/videonode-smoke-...` line (printed to stderr).
- That directory contains `streams.toml`, `server.log`, and the built binary — point the user there for postmortem.

## When NOT to use

- Frontend-only changes — use `cd ui && pnpm typecheck && pnpm build` instead.
- Quick iteration without functional changes — the suite takes ~30-90s depending on hardware.
- Running on a host where the production videonode is live on default ports — the suite uses ephemeral ports so this is fine, but be aware FFmpeg will be spawned for the test stream.

## Adding to the suite

New checks drop into `test/smoke/` as additional `Test*` functions in any `*_test.go` file with `//go:build smoke` at the top. The package-level handles `baseURL`, `httpPort`, `rtspPort`, `srtPort`, `authUser`, `authPass`, `runDir`, and `expectedEncoderFamily` are available everywhere. The helpers `newAuthReq`, `doExpect`, `decodeJSON`, `waitForStreamRunning`, `probeCodec`, and `dumpServerLogTail` are the building blocks.
