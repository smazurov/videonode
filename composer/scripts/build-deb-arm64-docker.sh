#!/usr/bin/env bash
# build-deb-arm64-docker.sh — spawn arm64v8/debian:trixie locally and run
# build-deb-arm64.sh inside it. No rig required (qemu emulates aarch64
# on x86_64). CI doesn't use this wrapper — it gets the container from
# GitHub Actions' `container:` directive and calls build-deb-arm64.sh
# directly. Same inner script in both cases.
#
# Env:
#   MODE=…                    Passed through to build-deb-arm64.sh.
#                              Default: release-deb (produces composer/$BUILD_DIR/*.deb).
#   BUILD_DIR=build/deb-arm64  Passed through to build-deb-arm64.sh.
#   ENGINE=docker|podman      Container engine (autodetected).
#   IMAGE=arm64v8/debian:trixie
set -euo pipefail

MODE="${MODE:-release-deb}"
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
IMAGE="${IMAGE:-arm64v8/debian:trixie}"
BUILD_DIR="${BUILD_DIR:-build/deb-arm64}"

ENGINE="${ENGINE:-}"
if [[ -z "$ENGINE" ]]; then
    if command -v docker >/dev/null; then ENGINE=docker
    elif command -v podman >/dev/null; then ENGINE=podman
    else echo "no docker or podman on PATH" >&2; exit 1
    fi
fi

if [[ "$(uname -m)" != "aarch64" && "$(uname -m)" != "arm64" ]]; then
    if ! "$ENGINE" run --rm --privileged multiarch/qemu-user-static --reset -p yes >/dev/null 2>&1; then
        echo "warning: could not register qemu-user-static; build may fail if host kernel lacks binfmt_misc for arm64" >&2
    fi
fi

echo ">>> $ENGINE run $IMAGE (MODE=$MODE)"

# apt inside the container requires root, but we don't want root-owned
# artifacts on the host. Run as root inside, chown back to the invoker
# at the end (even on failure).
HOST_UID="$(id -u)"
HOST_GID="$(id -g)"

"$ENGINE" run --rm --platform linux/arm64 \
    -v "$ROOT:/work" \
    -w /work \
    -e MODE="$MODE" \
    -e BUILD_DIR="$BUILD_DIR" \
    -e HOST_UID="$HOST_UID" \
    -e HOST_GID="$HOST_GID" \
    "$IMAGE" bash -eu -c '
        trap "chown -R \"$HOST_UID:$HOST_GID\" composer/build 2>/dev/null || true" EXIT
        composer/scripts/build-deb-arm64.sh
    '

if [[ "$MODE" == "release-deb" ]]; then
    echo
    echo "=== artifacts ==="
    ls -1 "$ROOT/composer/$BUILD_DIR"/*.deb
fi
