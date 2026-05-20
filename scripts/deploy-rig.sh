#!/usr/bin/env bash
# Build UI + arm64 daemon, push to rig, swap atomically, restart systemd-user.
# Env: SKIP_UI=1, SKIP_RESTART=1, RIG=user@host, REMOTE_PATH=/abs/path
set -euo pipefail

RIG="${RIG:-orangepi@orangepi5-ultra.lan}"
REMOTE_PATH="${REMOTE_PATH:-/home/orangepi/.local/bin/videonode}"
REMOTE_TMP="${REMOTE_PATH}.new"
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT}"

if [[ "${SKIP_UI:-0}" != "1" ]]; then
  echo ">>> ui: pnpm build"
  pnpm --prefix ui build
fi

echo ">>> go build linux/arm64 (-tags ui_embed)"
mkdir -p bin
GOOS=linux GOARCH=arm64 go build -tags ui_embed -o bin/videonode-arm64 .

echo ">>> rsync to ${RIG}:${REMOTE_TMP}"
rsync -a --info=progress2 bin/videonode-arm64 "${RIG}:${REMOTE_TMP}"

echo ">>> install + restart on ${RIG}"
ssh "${RIG}" bash -s <<EOF
set -euo pipefail
chmod +x "${REMOTE_TMP}"
mv "${REMOTE_TMP}" "${REMOTE_PATH}"
if [[ "${SKIP_RESTART:-0}" != "1" ]]; then
  systemctl --user restart videonode
  sleep 0.8
  systemctl --user is-active videonode
fi
EOF
echo "done"
