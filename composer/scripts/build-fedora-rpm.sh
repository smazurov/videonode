#!/usr/bin/env bash
# build-fedora-rpm.sh — local equivalent of the CI RPM job. Builds
# Release, runs tests, packs RPM. For hand-rolling release artifacts when
# CI is offline.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

if ! command -v rpmbuild >/dev/null; then
    echo "build-fedora-rpm.sh: rpmbuild not found; dnf install rpm-build" >&2
    exit 1
fi

cmake -B build -S . -G Ninja -DCMAKE_BUILD_TYPE=Release
cmake --build build
ctest --test-dir build --output-on-failure
(cd build && cpack -G RPM)

echo
echo "=== artifact ==="
ls -1 build/*.rpm
