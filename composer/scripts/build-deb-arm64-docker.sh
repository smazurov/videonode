#!/usr/bin/env bash
# build-deb-arm64-docker.sh — reproduce the deb-arm64 CI lane locally inside
# arm64v8/debian:trixie. No rig required (qemu emulates aarch64 on x86_64).
# Produces composer/build/deb-arm64/*.deb on the host.
#
# Env:
#   ENGINE=docker|podman      Container engine (autodetected).
#   IMAGE=arm64v8/debian:trixie
#   BUILD_DIR=build/deb-arm64  (relative to composer/)
set -euo pipefail

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

echo ">>> $ENGINE run $IMAGE → $BUILD_DIR/"

# apt inside the container requires root, but we don't want root-owned
# artifacts on the host. Run the build as root inside the container, then
# chown the build dir back to the invoking user at the end (even on failure).
HOST_UID="$(id -u)"
HOST_GID="$(id -g)"

"$ENGINE" run --rm --platform linux/arm64 \
    -v "$ROOT:/work" \
    -w /work \
    -e BUILD_DIR="$BUILD_DIR" \
    -e HOST_UID="$HOST_UID" \
    -e HOST_GID="$HOST_GID" \
    "$IMAGE" bash -eu -c '
        trap "chown -R \"$HOST_UID:$HOST_GID\" composer/$BUILD_DIR 2>/dev/null || true" EXIT
        apt-get update -qq
        DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
            cmake ninja-build pkg-config g++ ca-certificates curl git \
            libegl-dev libgles-dev libgbm-dev libdrm-dev \
            libturbojpeg0-dev dpkg-dev \
            libgrpc++-dev libprotobuf-dev protobuf-compiler protobuf-compiler-grpc
        composer/scripts/install-rockchip-libs.sh
        cd composer
        cmake -B "$BUILD_DIR" -S . -G Ninja -DCMAKE_BUILD_TYPE=Release
        cmake --build "$BUILD_DIR"
        ctest --test-dir "$BUILD_DIR" --output-on-failure
        (cd "$BUILD_DIR" && cpack -G DEB)
    '

echo
echo "=== artifacts ==="
ls -1 "$ROOT/composer/$BUILD_DIR"/*.deb
