#!/usr/bin/env bash
# Host-side smoke for the multi-fourcc EGL import path. Loops over
# NV12 / NV16 / NV24 / BG24, runs videonode-composer + scm-feeder for each,
# converts the first canvas frame to PNG. No rig involved.
#
# Outputs:
#   /tmp/canvas-<fourcc>.bgra    raw canvas dump
#   /tmp/canvas-<fourcc>.png     first frame, eyeball-checkable
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
COMPOSER="${ROOT}/composer/build/host/videonode-composer"
FEEDER="${ROOT}/bin/scm-feeder"
W=320 H=240 FPS=10 SECS=4

[ -x "$COMPOSER" ] || { echo "missing $COMPOSER — build composer first"; exit 1; }
[ -x "$FEEDER" ]   || { echo "missing $FEEDER — run 'make scm-feeder' first"; exit 1; }

results=()
for FMT in NV12 NV16 NV24 BG24; do
  SOCK="/tmp/srcA-${FMT}.sock"
  BGRA="/tmp/canvas-${FMT}.bgra"
  PNG="/tmp/canvas-${FMT}.png"
  LOG_COMPOSER="/tmp/composer-${FMT}.log"
  LOG_FEED="/tmp/feeder-${FMT}.log"

  rm -f "$SOCK" "$BGRA" "$PNG" "$LOG_COMPOSER" "$LOG_FEED"

  # Composer in background; redirect BGRA to file, stderr to log.
  nohup "$COMPOSER" --canvas-w $W --canvas-h $H --fps $FPS --seconds $SECS \
      --no-source-b --source-a-scm-path "$SOCK" \
      > "$BGRA" 2> "$LOG_COMPOSER" &
  COMPOSER_PID=$!

  # Wait for socket up
  for i in $(seq 1 30); do
    [ -S "$SOCK" ] && break
    sleep 0.1
  done

  # Drive it
  "$FEEDER" -synthetic -format-out "$FMT" -w $W -h $H -fps $FPS \
      -socket "$SOCK" -duration 3s > "$LOG_FEED" 2>&1 || true

  wait "$COMPOSER_PID" 2>/dev/null || true

  if [ ! -s "$BGRA" ]; then
    echo "$FMT FAIL: empty canvas (see $LOG_COMPOSER)"
    results+=("$FMT FAIL")
    continue
  fi

  # Convert first frame to PNG. BGRA byte order, canvas size known.
  ffmpeg -hide_banner -loglevel error -y \
      -f rawvideo -pix_fmt bgra -s ${W}x${H} -r $FPS -i "$BGRA" \
      -frames:v 1 -update 1 "$PNG"

  frames=$(($(stat -c%s "$BGRA") / (W*H*4)))
  results+=("$FMT ok frames=$frames -> $PNG")
done

echo
echo "=== summary ==="
for line in "${results[@]}"; do echo "  $line"; done
