---
name: testenv-down
description: Tear down a videonode test environment created by /testenv-up. Default is the current session's env. Pass an env id to target a specific one. Use when the user is done testing or asks to "tear down" / "stop" / "clean up" the env.
arguments:
  - env_id
allowed-tools:
  - Bash(testenv:*)
---

# /testenv-down

Stop a videonode test env, release its slot, and release any device leases it holds.

## Current inventory (live, before teardown)

!`testenv list 2>/dev/null || echo "(testenv binary not installed yet)"`

## Tear down

!`testenv down ${1:-} 2>&1`

If no `$env_id` is passed, this targets the env owned by the current Claude session. If the session owns no env, the command is a no-op and prints a friendly note — surface that to the user; don't invent a fix.
