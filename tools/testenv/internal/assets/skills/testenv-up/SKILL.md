---
name: testenv-up
description: Spin up a test environment in the current worktree and return its URL. Use when the user asks to "set up a test env", "start a test env", "spin up for testing", or similar. Coordinates with other parallel Claude sessions via the testenv registry so port and device leases never collide. Do NOT tear down the env after testing — leave it running unless the user asks.
arguments:
  - locks
allowed-tools:
  - Bash(testenv:*)
---

# /testenv-up

Bring up an isolated test environment per `.testenv.toml`.

## Arguments

- `$locks` — optional, comma-separated exclusive resource locks (e.g. `device:/dev/video0`). Omit for a no-device test env.

## Current inventory (live)

!`testenv list 2>/dev/null || echo "(testenv binary not installed yet)"`

## Bring up the env

!`testenv up ${locks:+--lock ${locks//,/ --lock }} 2>&1`

After `up` returns, the URL printed is the env you should poke at. Tell the user the URL plainly. If `up` fails with a lock conflict, the error message names the holding env/worktree/pid — surface that verbatim so the user can decide whether to take it over or wait.

If `up` succeeds and a `device:` lock was acquired, immediately fetch one frame to confirm the capture is real (not a green placeholder):

```
ffmpeg -i <rtsp_url from output> -frames:v 1 -y /tmp/testenv-firstframe.png 2>&1 | tail -5
```

Then describe what you saw so a green-placeholder regression is caught at env-up time, not three turns later.

**Leave the env running after testing.** Do not call testenv-down unless the user explicitly asks.
