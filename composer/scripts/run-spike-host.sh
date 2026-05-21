#!/usr/bin/env bash
# Run composer-spike on the local Fedora dev machine: lavfi sources for both
# slots (no real V4L2 needed), composer writes BGRA to stdout, ffmpeg encodes
# with libx264 and pushes RTSP to a local mediamtx. View from another terminal
# with `ffplay -rtsp_transport tcp rtsp://127.0.0.1:8554/spike`.
set -euo pipefail

CW="${CANVAS_W:-1280}"
CH="${CANVAS_H:-720}"
FPS="${CANVAS_FPS:-30}"
URL="${RTSP_URL:-rtsp://127.0.0.1:8554/spike}"

SPIKE_BIN="${SPIKE_BIN:-$(dirname "$0")/../build/host/composer-spike}"
DRM_DEVICE="${DRM_DEVICE:-/dev/dri/renderD128}"

if [ ! -x "${SPIKE_BIN}" ]; then
  echo "composer-spike not built at ${SPIKE_BIN}" >&2
  echo "build with: make spike-test  (from worktree root)" >&2
  exit 1
fi

# mediamtx must be running locally for the RTSP push to succeed. Start one
# in the background if not already.
if ! pgrep -x mediamtx >/dev/null; then
  if command -v mediamtx >/dev/null; then
    mediamtx >/tmp/mediamtx.log 2>&1 &
    MTX_PID=$!
    echo "started mediamtx pid=${MTX_PID} (log: /tmp/mediamtx.log)"
    sleep 0.5
    trap 'kill ${MTX_PID} 2>/dev/null || true' EXIT INT TERM
  else
    echo "mediamtx not on PATH. Install with: sudo dnf install mediamtx" >&2
    echo "or download from https://github.com/bluenviron/mediamtx/releases" >&2
    exit 1
  fi
fi

echo "composer pipeline: ${CW}x${CH}@${FPS} -> ${URL}"

"${SPIKE_BIN}" \
    --drm-device "${DRM_DEVICE}" \
    --canvas-w "${CW}" --canvas-h "${CH}" --fps "${FPS}" \
    --source-a-testsrc --source-a-width "${CW}" --source-a-height "${CH}" --source-a-fps "${FPS}" \
    --source-b-testsrc --source-b-width "${CW}" --source-b-height "${CH}" --source-b-fps "${FPS}" \
| ffmpeg \
    -hide_banner -loglevel warning \
    -f rawvideo -pix_fmt bgra -s "${CW}x${CH}" -framerate "${FPS}" -i pipe:0 \
    -c:v libx264 -preset ultrafast -tune zerolatency \
    -profile:v high -level:v 5.2 \
    -g 60 -bf 0 -b:v 4M \
    -rtsp_transport tcp -f rtsp "${URL}"
