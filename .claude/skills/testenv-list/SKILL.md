---
name: testenv-list
description: Show the current videonode test-env inventory across all parallel Claude sessions on this machine. Use when the user asks "what test envs are running", "who has the device", "what's on :8090", or before deciding whether to spin up a new env.
allowed-tools:
  - Bash(testenv:*)
---

# /testenv-list

Live inventory of test environments registered with testenv on this host. The list reflects all parallel Claude sessions, not just the current one.

!`testenv list 2>/dev/null || echo "(testenv binary not installed yet — run /testenv install once to set it up)"`

Read the table and tell the user plainly: which slots are taken, by which worktree/session, holding which devices, with which URLs. If the user is about to start a new env, point out which slots are free.
