#!/usr/bin/env bash
# build-deb-arm64.sh — runs inside an arm64 Debian trixie environment
# (either spawned locally via build-deb-arm64-docker.sh, or supplied by
# GH Actions via the `container:` directive). Installs deps and drives
# cmake for the requested MODE.
#
# MODE=release-nfpm (default) Release build (tests OFF) → cmake --install to staging → strip
#                              Artifact: composer/$BUILD_DIR/staging/bin/videonode-*
# MODE=dev                    cmake --preset dev → build → ctest
# MODE=dev-asan               cmake --preset dev-asan → build → ctest (ASan/UBSan)
# MODE=fuzz                   cmake --preset fuzz (clang) → build the libFuzzer
#                              harness → time-boxed campaign over the seed corpus
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
    release-nfpm|dev|dev-asan|fuzz|lint) ;;
    *) echo "build-deb-arm64.sh: unknown MODE='$MODE'" >&2; exit 2 ;;
esac

# libFuzzer campaign budget (seconds); override to run longer in nightly CI.
FUZZ_MAX_TOTAL_TIME="${FUZZ_MAX_TOTAL_TIME:-60}"

SUDO="${SUDO:-}"
if [[ $EUID -ne 0 ]] && [[ -z "$SUDO" ]] && command -v sudo >/dev/null; then
    SUDO=sudo
fi

# Per-mode extras. gtest/gmock are test-only — only the modes that build and run
# the suite (dev, dev-asan, lint configures via the dev preset which sets
# BUILD_TESTS=ON) get them. release-nfpm builds with BUILD_TESTS=OFF and instead
# pulls dpkg-dev for dpkg-shlibdeps (runtime-dep generation, see scripts/gen-deb-depends.sh).
case "$MODE" in
    lint)         extra_pkgs=(clang-format clang-tidy libgtest-dev libgmock-dev) ;;
    dev|dev-asan) extra_pkgs=(libgtest-dev libgmock-dev) ;;
    # fuzz configures with BUILD_TESTS=OFF, so no gtest; clang-19 is the trixie
    # default and its fuzzer/sanitizer runtimes live in the matching -19 dev pkgs.
    fuzz)         extra_pkgs=(clang libclang-rt-19-dev libfuzzer-19-dev) ;;
    release-nfpm) extra_pkgs=(dpkg-dev) ;;
    *)            extra_pkgs=() ;;
esac

$SUDO apt-get update -qq
DEBIAN_FRONTEND=noninteractive $SUDO apt-get install -y --no-install-recommends \
    cmake ninja-build pkg-config g++ ca-certificates curl git \
    libegl-dev libgles-dev libgbm-dev libdrm-dev \
    libturbojpeg0-dev \
    libplacebo-dev libvulkan-dev \
    libabsl-dev \
    libgrpc++-dev libprotobuf-dev protobuf-compiler protobuf-compiler-grpc \
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
        # Shipping build: tests OFF (the default; explicit for clarity), so
        # gtest is never needed and there are no tests to run.
        cmake -B "$BUILD_DIR" -S . -G Ninja -DCMAKE_BUILD_TYPE=Release -DBUILD_TESTS=OFF
        cmake --build "$BUILD_DIR"
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
    fuzz)
        # Only the harness target compiles under clang — not the whole GL/gRPC
        # tree — so this stays fast and independent of clang's view of the rest.
        cmake --preset fuzz -DBUILD_TESTS=OFF
        cmake --build --preset fuzz --target fuzz_dmabuf_header_decode
        seeds=fuzz/corpus/dmabuf_header_decode
        workdir=build/fuzz/fuzz/fuzz_dmabuf_header_decode.corpus
        mkdir -p "$workdir"
        ./build/fuzz/fuzz/fuzz_dmabuf_header_decode "$workdir" "$seeds" \
            -max_total_time="$FUZZ_MAX_TOTAL_TIME" -timeout=10 -error_exitcode=77
        ;;
    lint)
        cmake --preset dev
        cmake --build --preset dev --target lint
        cmake --build --preset dev --target tidy-diff
        ;;
esac
