#!/usr/bin/env bash
# Build the composer on the rig via ssh using cmake + ninja.
#
# Operational notes:
# - ssh keepalive drops the connection in ~15s if the rig stops
#   responding, instead of sitting on a dead session indefinitely.
# - STOPS videonode.service first so the compile isn't competing with
#   the HDMI capture pipeline for CPU + DRAM. Caller restarts it.
# - JOBS=${JOBS:-2} parallel jobs by default. RK3588 has 8 cores but
#   full-parallel compile alongside anything else has wedged the box.
#   Default 4; override with JOBS=8 if the rig is idle.
# - Release build by default (faster + smaller). Override with
#   BUILD_TYPE=Debug for sanitizer-style builds.
# - KEEP_SERVICE=1 leaves videonode.service running across the build
#   (use only when you know the rig is idle).
# - VERSION stamps the binaries (vn::kVersion). The rig is a synced tree
#   with no .git, so `git describe` there falls back to 0.1.0. We compute
#   it here on the dev box (which has .git) and pass it through, matching
#   what a native dev-box build stamps. Override by exporting VERSION.
#
# First time: configures cmake. Subsequent runs: incremental ninja build.
set -euo pipefail

RIG="${RIG:-orangepi}"
DST_DIR="${DST_DIR:-/home/orangepi/composer}"
JOBS="${JOBS:-4}"
BUILD_TYPE="${BUILD_TYPE:-Release}"
KEEP_SERVICE="${KEEP_SERVICE:-0}"
VERSION="${VERSION:-$(git -C "$(dirname "${BASH_SOURCE[0]}")" describe --tags --always --dirty 2>/dev/null || true)}"

echo ">>> build-on-rig: rig=${RIG} jobs=${JOBS} build_type=${BUILD_TYPE} keep_service=${KEEP_SERVICE} version=${VERSION:-<unset>}"

ssh -o ServerAliveInterval=5 -o ServerAliveCountMax=3 \
    -o LogLevel=ERROR \
    "${RIG}" \
    "DST_DIR='${DST_DIR}' JOBS='${JOBS}' BUILD_TYPE='${BUILD_TYPE}' KEEP_SERVICE='${KEEP_SERVICE}' VERSION='${VERSION}' bash -s" <<'REMOTE'
set -euo pipefail
export VERSION

if [ "${KEEP_SERVICE}" != "1" ]; then
  echo ">>> stopping videonode.service on rig"
  systemctl --user stop videonode.service 2>/dev/null || true
fi

cd "${DST_DIR}"

if [ -f build/meson-info/meson-info.json ]; then
  echo ">>> build/ has a meson layout; clearing for cmake"
  rm -rf build
fi

if [ ! -f build/CMakeCache.txt ]; then
  echo ">>> cmake configure (${BUILD_TYPE})"
  cmake -S . -B build -G Ninja -DCMAKE_BUILD_TYPE="${BUILD_TYPE}"
fi

echo ">>> ninja build (-j${JOBS})"
cmake --build build -j "${JOBS}"

echo
echo "=== built targets ==="
find build/src/bin -maxdepth 1 -type f -executable -printf "  %f\n" 2>/dev/null | sort
REMOTE
