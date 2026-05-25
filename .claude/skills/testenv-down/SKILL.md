---
name: testenv-down
description: Tear down a test environment. ONLY use when the user explicitly asks to tear down, stop, or clean up the env. Do NOT tear down between test steps or when you think you're "done" — envs are cheap and restarting wastes time. Leave them running unless told otherwise.
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
