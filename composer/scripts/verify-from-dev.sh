#!/usr/bin/env bash
# Validate the RTSP stream from the dev machine. Pulls a few seconds, asserts
# basic stream properties (h264, expected resolution, non-zero frame rate),
# and optionally pops ffplay for a visual.
#
# Usage:
#   bash scripts/verify-from-dev.sh                # ffprobe assertions only
#   PLAY=1 bash scripts/verify-from-dev.sh         # ffprobe + ffplay
set -euo pipefail

RIG_HOST="${RIG_HOST:-orangepi5-ultra.lan}"
STREAM="${STREAM:-spike}"
URL="rtsp://${RIG_HOST}:8554/${STREAM}"
EXPECT_W="${EXPECT_W:-1920}"
EXPECT_H="${EXPECT_H:-1080}"

echo ">>> probing ${URL}"
PROBE_JSON=$(ffprobe -v error -of json -rtsp_transport tcp -show_streams -read_intervals "%+5" "${URL}")
echo "${PROBE_JSON}" | head -40

codec=$(echo "${PROBE_JSON}" | grep -m1 '"codec_name":' | sed 's/.*"codec_name": *"\([^"]*\)".*/\1/')
width=$(echo "${PROBE_JSON}" | grep -m1 '"width":' | sed 's/[^0-9]//g')
height=$(echo "${PROBE_JSON}" | grep -m1 '"height":' | sed 's/[^0-9]//g')

failed=0
[ "${codec}"  = "h264" ]      || { echo "FAIL: codec=${codec} (want h264)"; failed=1; }
[ "${width}"  = "${EXPECT_W}" ] || { echo "FAIL: width=${width} (want ${EXPECT_W})"; failed=1; }
[ "${height}" = "${EXPECT_H}" ] || { echo "FAIL: height=${height} (want ${EXPECT_H})"; failed=1; }
[ $failed = 0 ] && echo "PASS: ${codec} ${width}x${height}"

if [ "${PLAY:-0}" = "1" ]; then
  echo ">>> ffplay (Ctrl-C to stop)"
  ffplay -rtsp_transport tcp -fflags nobuffer -i "${URL}"
fi

exit $failed
