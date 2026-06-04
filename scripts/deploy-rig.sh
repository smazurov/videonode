#!/usr/bin/env bash
# Load-bearing rig deploy orchestrator. The /deploy-rig skill DRIVES this
# script — it is the source of truth for the deploy mechanism, not the prose.
#
# It encodes the CURRENT rig contract. If any of this drifts, fix it HERE
# (the skill points the agent at this file to inspect + amend before running):
#   - videonode.service is a systemd *system* unit (NOT `--user`)
#   - binaries live in /usr/bin (root-owned -> sudo -S, password piped)
#   - native helpers come from <rig>:${RIG_BIN_DIR} (built by build-on-rig.sh)
#   - the go supervisor is staged to <rig>:${STAGE_PATH} by build-go-arm64.sh
#   - config is /etc/videonode/{config.toml,streams.toml}
#
# Flow: preflight -> [host sanity build] -> stop service
#       -> [composer sync + rig build] -> [go UI+cross-build+stage]
#       -> install into /usr/bin -> start service -> post-deploy preflight.
# The service is stopped once (frees CPU for the rig build AND releases the
# text segment so the /usr/bin cp won't hit "Text file busy") and started once.
#
# Env:
#   RIG=user@host        ssh target            (default: orangepi)
#   SUDO_PASS=...        piped to `sudo -S`    (default: orangepi)
#   JOBS=N               rig build parallelism (default: 8)
#   SKIP_HOST_BUILD=1    skip the host sanitizer sanity build
#   SKIP_COMPOSER=1      skip composer sync + rig build + native-helper install
#   SKIP_GO=1            skip go UI+cross-build+install (composer-only deploy)
#   RIG_BIN_DIR=path     native bins on the rig (default: /home/orangepi/composer/build/src/bin)
#   STAGE_PATH=path      staged go binary on the rig (default: /tmp/videonode-arm64.staging)
set -euo pipefail

RIG="${RIG:-orangepi}"
SUDO_PASS="${SUDO_PASS:-REDACTED}"
RIG_BIN_DIR="${RIG_BIN_DIR:-/home/orangepi/composer/build/src/bin}"
STAGE_PATH="${STAGE_PATH:-/tmp/videonode-arm64.staging}"
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT}"

# Fail fast on a dead/auth-stalled rig instead of hanging mid-deploy.
SSH=(ssh -o BatchMode=yes -o ConnectTimeout=10 "${RIG}")

echo "=== 0. preflight ==="
# Preflight aborts the whole deploy if the rig is unreachable (its ssh fails
# under -o BatchMode), so connectivity is gated before we stop anything.
bash scripts/deploy-preflight.sh

if [[ "${SKIP_HOST_BUILD:-0}" != "1" ]]; then
  echo "=== 1. host sanity build (composer dev preset) ==="
  ( cd composer && cmake --preset dev >/dev/null && cmake --build --preset dev )
fi

echo "=== 2. stop videonode.service ==="
"${SSH[@]}" "sudo systemctl stop videonode.service"

if [[ "${SKIP_COMPOSER:-0}" != "1" ]]; then
  echo "=== 3. composer sync + rig build ==="
  composer/scripts/sync-to-rig.sh
  RIG="${RIG}" JOBS="${JOBS:-8}" KEEP_SERVICE=1 composer/scripts/build-on-rig.sh
fi

if [[ "${SKIP_GO:-0}" != "1" ]]; then
  echo "=== 4. go UI + cross-build + stage ==="
  RIG="${RIG}" STAGE_PATH="${STAGE_PATH}" bash scripts/build-go-arm64.sh
fi

echo "=== 5. install into /usr/bin + start service ==="
# The remote runs under `set -euo pipefail`, so a failed cp/chmod aborts. After
# start we hard-gate on `is-active`: if the service did not come up we dump the
# journal and exit non-zero so the orchestrator FAILs loudly instead of
# reporting a green deploy over a dead service.
"${SSH[@]}" "
  set -euo pipefail
  s() { sudo \"\$@\"; }
  if [[ '${SKIP_GO:-0}' != '1' ]]; then s cp -f '${STAGE_PATH}' /usr/bin/videonode; fi
  if [[ '${SKIP_COMPOSER:-0}' != '1' ]]; then
    s cp -f '${RIG_BIN_DIR}'/videonode-source   /usr/bin/
    s cp -f '${RIG_BIN_DIR}'/videonode-sink     /usr/bin/
    s cp -f '${RIG_BIN_DIR}'/videonode-composer /usr/bin/
  fi
  s chmod +x /usr/bin/videonode /usr/bin/videonode-source /usr/bin/videonode-sink /usr/bin/videonode-composer
  s systemctl start videonode.service
  sleep 2
  if ! systemctl is-active --quiet videonode.service; then
    echo 'FAIL: videonode.service is not active after start' >&2
    s journalctl -u videonode.service -n 50 --no-pager >&2 || true
    exit 1
  fi
"

echo "=== 6. post-deploy preflight (expect service=active; decision: UP-TO-DATE) ==="
sleep 1
bash scripts/deploy-preflight.sh
echo ">>> deploy-rig done — probe a live stream with: STREAM=lyra composer/scripts/verify-from-dev.sh"
