#!/usr/bin/env bash
# build-deb-arm64.sh — runs inside an arm64 Debian trixie environment
# (either spawned locally via build-deb-arm64-docker.sh, or supplied by
# GH Actions via the `container:` directive). Installs deps and drives
# cmake for the requested MODE.
#
# MODE=release-nfpm (default) Release build → ctest → cmake --install to staging → strip
#                              Artifact: composer/$BUILD_DIR/staging/bin/videonode-*
# MODE=dev                    cmake --preset dev → build → ctest
# MODE=dev-asan               cmake --preset dev-asan → build → ctest (ASan/UBSan)
# MODE=lint                   cmake --preset dev → build lint + tidy-diff
#                              Needs origin/main fetched (CI: fetch-depth: 0).
#
# Env:
#   MODE=…                    See above.
#   BUILD_DIR=build/deb-arm64  (release-nfpm only)
set -euo pipefail

MODE="${MODE:-release-nfpm}"
BUILD_DIR="${BUILD_DIR:-build/deb-arm64}"

case "$MODE" in
    release-nfpm|dev|dev-asan|lint) ;;
    *) echo "build-deb-arm64.sh: unknown MODE='$MODE'" >&2; exit 2 ;;
esac

SUDO="${SUDO:-}"
if [[ $EUID -ne 0 ]] && [[ -z "$SUDO" ]] && command -v sudo >/dev/null; then
    SUDO=sudo
fi

case "$MODE" in
    lint) extra_pkgs=(clang-format clang-tidy) ;;
    *)    extra_pkgs=() ;;
esac

$SUDO apt-get update -qq
DEBIAN_FRONTEND=noninteractive $SUDO apt-get install -y --no-install-recommends \
    cmake ninja-build pkg-config g++ ca-certificates curl git \
    libegl-dev libgles-dev libgbm-dev libdrm-dev \
    libturbojpeg0-dev \
    libplacebo-dev libvulkan-dev \
    libabsl-dev \
    libgrpc++-dev libprotobuf-dev protobuf-compiler protobuf-compiler-grpc \
    libgtest-dev libgmock-dev \
    "${extra_pkgs[@]}"

# Locate composer/ from this script's location so callers don't need to cwd.
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
"$ROOT/composer/scripts/install-rockchip-libs.sh"

# Workspace may be host-mounted (local docker) with a different owner;
# git refuses to operate without this. No-op when the repo is already
# owned by the current user.
git config --global --add safe.directory "$ROOT"

cd "$ROOT/composer"
case "$MODE" in
    release-nfpm)
        cmake -B "$BUILD_DIR" -S . -G Ninja -DCMAKE_BUILD_TYPE=Release
        cmake --build "$BUILD_DIR"
        ctest --test-dir "$BUILD_DIR" --output-on-failure
        cmake --install "$BUILD_DIR" --prefix "$BUILD_DIR/staging"
        find "$BUILD_DIR/staging/bin" -type f -executable -exec strip {} +
        ;;
    dev)
        cmake --preset dev
        cmake --build --preset dev
        ctest --preset dev --output-on-failure
        ;;
    dev-asan)
        cmake --preset dev-asan
        cmake --build --preset dev-asan
        ASAN_OPTIONS=detect_leaks=1 UBSAN_OPTIONS=print_stacktrace=1:halt_on_error=1 \
            ctest --preset dev-asan --output-on-failure
        ;;
    lint)
        cmake --preset dev
        cmake --build --preset dev --target lint
        cmake --build --preset dev --target tidy-diff
        ;;
esac
