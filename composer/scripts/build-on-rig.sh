#!/usr/bin/env bash
# Build the composer on the rig via ssh using cmake + ninja.
# First time: configures cmake. Subsequent runs: incremental ninja build.
# If the build dir contains a stale meson configuration (the project used
# to be meson), we wipe it before reconfiguring.
set -euo pipefail

RIG="${RIG:-orangepi@orangepi5-ultra.lan}"
DST_DIR="${DST_DIR:-/home/orangepi/composer}"

ssh "${RIG}" "bash -lc 'set -euo pipefail
  cd ${DST_DIR}
  # Wipe a stale meson build directory if one is left over.
  if [ -f build/meson-info/meson-info.json ]; then
    echo \"build/ has a meson layout; clearing for cmake\"
    rm -rf build
  fi
  if [ ! -f build/CMakeCache.txt ]; then
    cmake -S . -B build -G Ninja
  fi
  cmake --build build
  echo
  echo \"=== built targets ===\"
  find build -maxdepth 1 -type f -executable -printf \"  %f\n\" | sort
'"
