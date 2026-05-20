#!/usr/bin/env bash
# Rsync the composer-spike tree to the rig under ~/composer-spike/.
# No build; only file sync. Use scripts/build-on-rig.sh for that.
set -euo pipefail

RIG="${RIG:-orangepi@orangepi5-ultra.lan}"
SRC_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DST_DIR="${DST_DIR:-/home/orangepi/composer-spike}"

echo ">>> syncing ${SRC_DIR} -> ${RIG}:${DST_DIR}"
rsync -a --delete \
  --exclude 'build/' \
  --exclude '.cache/' \
  "${SRC_DIR}/" "${RIG}:${DST_DIR}/"
echo "ok"
