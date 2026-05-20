#!/usr/bin/env bash
# Run the multi-fourcc smoke ON THE RIG (assumed to be already on it).
# Loops over NV12 / NV16 / NV24 / BG24, runs composer-spike + scm-feeder
# against /dev/dma_heap/system + Mali-Panthor, captures one PNG per format.
set -euo pipefail

ROOT="/home/orangepi/composer-spike"
SPIKE="${ROOT}/build/composer-spike"
FEEDER="${ROOT}/build/scm-feeder"
W=320 H=240 FPS=10 SECS=4
DRM_DEV="${DRM_DEV:-/dev/dri/renderD130}"

[ -x "$SPIKE" ]  || { echo "missing $SPIKE";  exit 1; }
[ -x "$FEEDER" ] || { echo "missing $FEEDER"; exit 1; }

results=()
for FMT in NV12 NV16 NV24 BG24; do
  SOCK="/tmp/srcA-${FMT}.sock"
  BGRA="/tmp/canvas-${FMT}.bgra"
  PNG="/tmp/canvas-${FMT}.png"
  LOG_SPIKE="/tmp/spike-${FMT}.log"
  LOG_FEED="/tmp/feeder-${FMT}.log"

  rm -f "$SOCK" "$BGRA" "$PNG" "$LOG_SPIKE" "$LOG_FEED"

  nohup "$SPIKE" --canvas-w $W --canvas-h $H --fps $FPS --seconds $SECS \
      --drm-device "$DRM_DEV" \
      --no-source-b --source-a-scm-path "$SOCK" \
      > "$BGRA" 2> "$LOG_SPIKE" &
  SPIKE_PID=$!

  for i in $(seq 1 30); do
    [ -S "$SOCK" ] && break
    sleep 0.1
  done

  "$FEEDER" -synthetic -format-out "$FMT" -w $W -h $H -fps $FPS \
      -socket "$SOCK" -duration 3s > "$LOG_FEED" 2>&1 || true

  wait "$SPIKE_PID" 2>/dev/null || true

  if [ ! -s "$BGRA" ]; then
    results+=("$FMT FAIL: empty canvas")
    echo "--- $FMT spike log ---"; tail -8 "$LOG_SPIKE"
    continue
  fi

  ffmpeg -hide_banner -loglevel error -y \
      -f rawvideo -pix_fmt bgra -s ${W}x${H} -r $FPS -i "$BGRA" \
      -frames:v 1 -update 1 "$PNG"

  frames=$(($(stat -c%s "$BGRA") / (W*H*4)))
  results+=("$FMT ok frames=$frames -> $PNG")
done

echo
echo "=== summary ==="
for line in "${results[@]}"; do echo "  $line"; done
