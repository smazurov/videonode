---
name: testenv-up
description: Spin up a videonode test environment in the current worktree and return its URL. Use when the user asks to "set up a test env", "start a test env", "spin up videonode for testing", or similar. Coordinates with other parallel Claude sessions via the testenv registry so port and device leases never collide.
arguments:
  - target
  - source
allowed-tools:
  - Bash(testenv:*)
---

# /testenv-up

Bring up an isolated videonode test environment in the current worktree.

## Arguments

- `$target` — `host` (default) or `sbc`. `sbc` cross-compiles and ssh-spawns on `orangepi5-ultra.lan`.
- `$source` — `fake` (default, test_mode synthetic source) or `real` (requires a free `/dev/video0` lease).

## Current inventory (live)

!`testenv list 2>/dev/null || echo "(testenv binary not installed yet)"`

## Bring up the env

!`testenv up --target ${1:-host} --source ${2:-fake} 2>&1`

After `up` returns, the URL printed is the env you should poke at. Tell the user the URL plainly. If `up` fails with a device-lease conflict, the error message names the holding env/worktree/pid — surface that verbatim so the user can decide whether to take it over or wait.

If `up` succeeds and the user asked for `real` source, immediately fetch one frame to confirm the capture is real (not a green placeholder):

```
ffmpeg -i $RTSP_URL -frames:v 1 -y /tmp/testenv-firstframe.png 2>&1 | tail -5
```

Then describe what you saw (or use a screenshot helper if available) so a green-placeholder regression is caught at env-up time, not three turns later.
