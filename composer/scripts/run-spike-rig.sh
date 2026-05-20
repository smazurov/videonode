#!/usr/bin/env bash
# Run composer-spike on the rig: HDMI-IN (4K NV12) + Lyra (1080p MJPEG) ->
# GLES compose -> stdout BGRA -> ffmpeg h264_rkmpp -> RTSP -> mediamtx.
#
# Starts a local mediamtx on the rig if one isn't already running.
# View from the dev machine with:
#   ffplay -rtsp_transport tcp rtsp://orangepi5-ultra.lan:8554/spike

set -euo pipefail

RIG="${RIG:-orangepi@orangepi5-ultra.lan}"
SEC="${SECONDS_RUN:-0}"
CW="${CANVAS_W:-1920}"
CH="${CANVAS_H:-1080}"
FPS="${CANVAS_FPS:-30}"
NAME="${STREAM_NAME:-spike}"

echo ">>> launching on ${RIG}"
ssh -t "${RIG}" "bash -lc '
  set -euo pipefail
  cd /home/orangepi/composer-spike
  if [ ! -x build/composer-spike ]; then
    echo \"composer-spike not built; run scripts/build-on-rig.sh first\" >&2
    exit 1
  fi
  if [ ! -x /home/orangepi/mediamtx ]; then
    echo \"mediamtx not installed; run scripts/install-mediamtx.sh first\" >&2
    exit 1
  fi
  pkill -f \"/home/orangepi/mediamtx\" 2>/dev/null || true
  sleep 0.2
  /home/orangepi/mediamtx /home/orangepi/mediamtx.yml >/tmp/mediamtx.log 2>&1 &
  MTX_PID=\$!
  echo \"mediamtx pid=\$MTX_PID  log=/tmp/mediamtx.log\"
  sleep 0.5
  trap \"kill \$MTX_PID 2>/dev/null || true\" EXIT INT TERM
  ./build/composer-spike \
      --drm-device /dev/dri/renderD130 \
      --canvas-w ${CW} --canvas-h ${CH} --fps ${FPS} \
      --seconds ${SEC} \
  | ffmpeg \
      -hide_banner -loglevel warning \
      -f rawvideo -pix_fmt bgra -s ${CW}x${CH} -framerate ${FPS} -i pipe:0 \
      -c:v h264_rkmpp -profile:v high -level:v 5.2 -rc_mode VBR -b:v 6M -g 60 -bf 0 \
      -bsf:v dump_extra=freq=keyframe \
      -rtsp_transport tcp -f rtsp rtsp://127.0.0.1:8554/${NAME}
'"
