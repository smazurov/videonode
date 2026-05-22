#!/usr/bin/env bash
# Rsync the composer/ tree to the rig under ~/composer/.
# No build; only file sync. Use scripts/build-on-rig.sh for that.
set -euo pipefail

RIG="${RIG:-orangepi}"
SRC_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
REPO_ROOT="$(cd "${SRC_DIR}/.." && pwd)"
DST_DIR="${DST_DIR:-/home/orangepi/composer}"
PROTO_DST_DIR="${PROTO_DST_DIR:-/home/orangepi/proto}"

echo ">>> syncing ${SRC_DIR} -> ${RIG}:${DST_DIR}"
rsync -a --delete \
  --exclude 'build/' \
  --exclude '.cache/' \
  "${SRC_DIR}/" "${RIG}:${DST_DIR}/"

# The composer CMake expects proto schemas at ${CMAKE_SOURCE_DIR}/../proto
# (repo-root layout). Mirror the dir to the rig alongside composer/.
if [ -d "${REPO_ROOT}/proto" ]; then
  echo ">>> syncing ${REPO_ROOT}/proto -> ${RIG}:${PROTO_DST_DIR}"
  rsync -a --delete "${REPO_ROOT}/proto/" "${RIG}:${PROTO_DST_DIR}/"
fi
echo "ok"
