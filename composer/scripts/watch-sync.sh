#!/usr/bin/env bash
# Watch composer/ for file changes and rsync to the rig.
# Debounced: coalesces bursts of writes (e.g., editor save -> save) within 200ms.
# Stop with Ctrl-C or by killing the inotifywait process.
set -euo pipefail

RIG="${RIG:-orangepi}"
SRC_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DST_DIR="${DST_DIR:-/home/orangepi/composer}"
DEBOUNCE_MS="${DEBOUNCE_MS:-200}"

sync_once() {
  rsync -a --delete \
    --exclude 'build/' \
    --exclude '.cache/' \
    "${SRC_DIR}/" "${RIG}:${DST_DIR}/" >/dev/null
  printf '[sync] %s -> %s\n' "$(date +%H:%M:%S)" "${RIG}:${DST_DIR}"
}

# Initial sync so the rig matches before the watch starts.
sync_once

# Watch loop: any event triggers a debounced sync.
# `--monitor` keeps inotifywait alive; `--quiet` suppresses per-event chatter.
inotifywait --monitor --quiet --recursive \
  --event modify,create,delete,move,close_write \
  --exclude '(\.swp$|~$|/build/|/\.cache/)' \
  "${SRC_DIR}" \
| while read -r _; do
    # Debounce: drain any further events that arrive within DEBOUNCE_MS.
    # `read -t` returns 1 after the timeout with nothing read, ending the drain.
    while read -r -t "$(awk -v ms="${DEBOUNCE_MS}" 'BEGIN { print ms / 1000 }')" _; do
      :
    done
    sync_once
  done
