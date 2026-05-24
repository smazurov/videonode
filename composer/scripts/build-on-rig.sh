#!/usr/bin/env bash
# Build the composer on the rig via ssh using cmake + ninja.
#
# DEPRECATED for production deploy. Use
# `composer/scripts/build-deb-install-rig.sh` (Docker-built .deb installed
# via dpkg). Building on the rig while the hdmirx capture pipeline is
# running has been observed to destabilise the device.
#
# This script is retained only because `composer-smoke` reads its rig
# binaries from `$RIG_BUILD/src/bin/videonode-*` and the canonical .deb
# install lands at `/usr/bin/` (different layout). For smoke iteration on
# rig-only code paths, this is still the fastest way to refresh the
# scratch build dir.
#
# First time: configures cmake. Subsequent runs: incremental ninja build.
# If the build dir contains a stale meson configuration (the project used
# to be meson), we wipe it before reconfiguring.
set -euo pipefail

RIG="${RIG:-orangepi}"
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
