#!/usr/bin/env bash
# install-rockchip-libs.sh — fetch tsukumijima's prebuilt librga + librockchip-mpp
# arm64 .debs and install them. Used by CI lanes and by build-deb-arm64-docker.sh.
# Versions are pinned here; bump deliberately.
set -euo pipefail

LIBRGA_VER=2.2.0-1
LIBRGA_TAG=v2.2.0-1-20260121-2cffdf6
LIBMPP_VER=1.5.0-1
LIBMPP_TAG=v1.5.0-1-20260121-750e76e

rga="https://github.com/tsukumijima/librga-rockchip/releases/download/${LIBRGA_TAG}"
mpp="https://github.com/tsukumijima/mpp-rockchip/releases/download/${LIBMPP_TAG}"
urls=(
    "${rga}/librga2_${LIBRGA_VER}_arm64.deb"
    "${rga}/librga-dev_${LIBRGA_VER}_arm64.deb"
    "${mpp}/librockchip-mpp1_${LIBMPP_VER}_arm64.deb"
    "${mpp}/librockchip-mpp-dev_${LIBMPP_VER}_arm64.deb"
    "${mpp}/librockchip-vpu0_${LIBMPP_VER}_arm64.deb"
)

SUDO="${SUDO:-}"
if [[ $EUID -ne 0 ]] && [[ -z "$SUDO" ]] && command -v sudo >/dev/null; then
    SUDO=sudo
fi

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
for url in "${urls[@]}"; do
    curl -fsSL -o "$tmp/$(basename "$url")" "$url"
done
$SUDO dpkg -i "$tmp"/*.deb
pkg-config --modversion librga rockchip_mpp
