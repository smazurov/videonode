#!/usr/bin/env bash
# build-fedora-rpm.sh — builds Release, runs tests, installs to
# ~/.local/bin (the default prefix). For local dev on Fedora.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

cmake -B build -S . -G Ninja -DCMAKE_BUILD_TYPE=Release
cmake --build build
ctest --test-dir build --output-on-failure
cmake --install build

echo
echo "=== installed ==="
ls -l ~/.local/bin/videonode-{source,sink,composer} 2>/dev/null || echo "(check CMAKE_INSTALL_PREFIX)"
