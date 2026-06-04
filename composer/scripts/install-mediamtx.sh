#!/usr/bin/env bash
# Install mediamtx single-binary RTSP server on the rig.
# Reversible: `rm /home/orangepi/mediamtx` and stop the systemd-run unit if active.
set -euo pipefail

RIG="${RIG:-orangepi}"
VERSION="${MEDIAMTX_VERSION:-v1.9.3}"   # bump as needed

ssh "${RIG}" "bash -lc '
  set -euo pipefail
  if [ -x /home/orangepi/mediamtx ]; then
    echo \"mediamtx already installed at /home/orangepi/mediamtx\"
    /home/orangepi/mediamtx --version || true
    exit 0
  fi
  cd /tmp
  url=\"https://github.com/bluenviron/mediamtx/releases/download/${VERSION}/mediamtx_${VERSION}_linux_arm64v8.tar.gz\"
  echo \"downloading \$url\"
  curl -fL \"\$url\" -o mediamtx.tar.gz
  tar -xzf mediamtx.tar.gz mediamtx mediamtx.yml
  mv mediamtx /home/orangepi/mediamtx
  mv mediamtx.yml /home/orangepi/mediamtx.yml
  chmod +x /home/orangepi/mediamtx
  rm mediamtx.tar.gz
  /home/orangepi/mediamtx --version
'"
