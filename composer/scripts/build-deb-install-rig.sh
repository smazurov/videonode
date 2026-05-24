#!/usr/bin/env bash
# build-deb-install-rig.sh — build the videonode-native arm64 DEB inside an
# arm64v8/debian:trixie container, copy it to the rig, and install via dpkg.
#
# Replaces the older on-rig cmake/ninja build path
# (scripts/build-on-rig.sh), which is now retained only for smoke-test
# scratch builds — the canonical deploy uses this Docker-built .deb so the
# rig isn't compiling C++ while the hdmirx capture pipeline is running.
#
# Resulting install layout on the rig:
#   /usr/bin/videonode-{source,sink,composer}
#   ~/.config/systemd/user/videonode.service.d/native-pipeline.conf
#     (Environment= pointing the Go daemon at /usr/bin/videonode-*)
#
# Env:
#   RIG=orangepi              ssh target.
#   SUDO=sudo                 elevation tool on the rig.
#   SKIP_BUILD=0|1            reuse the most recent composer/build/deb-arm64/*.deb
#                             without rebuilding (faster iteration).
set -euo pipefail

RIG="${RIG:-orangepi}"
SUDO="${SUDO:-sudo}"
SKIP_BUILD="${SKIP_BUILD:-0}"

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
BUILD_DIR="${ROOT}/composer/build/deb-arm64"

if [ "$SKIP_BUILD" != "1" ]; then
    echo ">>> building arm64 .deb in Docker"
    "${ROOT}/composer/scripts/build-deb-arm64-docker.sh"
else
    echo ">>> SKIP_BUILD=1; reusing existing artifact"
fi

DEB="$(ls -1t "${BUILD_DIR}"/*.deb 2>/dev/null | head -1)"
if [ -z "$DEB" ]; then
    echo "no .deb in ${BUILD_DIR}" >&2
    exit 1
fi
DEB_NAME="$(basename "$DEB")"
echo ">>> artifact: $DEB"

echo ">>> copying to ${RIG}:/tmp/${DEB_NAME}"
scp "$DEB" "${RIG}:/tmp/${DEB_NAME}"

echo ">>> stopping videonode service + installing on rig"
ssh "${RIG}" "
set -euo pipefail
systemctl --user stop videonode.service 2>/dev/null || true
${SUDO} dpkg -i /tmp/${DEB_NAME}
mkdir -p \$HOME/.config/systemd/user/videonode.service.d
cat > \$HOME/.config/systemd/user/videonode.service.d/native-pipeline.conf <<'EOF'
[Service]
Environment=\"NATIVE_PIPELINE_SOURCE=/usr/bin/videonode-source\"
Environment=\"NATIVE_PIPELINE_SINK=/usr/bin/videonode-sink\"
Environment=\"NATIVE_PIPELINE_COMPOSER=/usr/bin/videonode-composer\"
EOF
systemctl --user daemon-reload
"

echo ">>> installed bins on ${RIG}:"
ssh "${RIG}" '
for b in videonode-source videonode-sink videonode-composer; do
    p=/usr/bin/$b
    if [ -x $p ]; then
        printf "    %s  %s\n" "$p" "$($p --version 2>/dev/null || echo "(--version failed)")"
    else
        printf "    %s  MISSING\n" "$p"
    fi
done
'

echo
echo ">>> done. Start the service with:"
echo "    ssh ${RIG} 'systemctl --user start videonode.service'"
echo ">>> or, if you also staged a Go supervisor at /tmp/videonode-arm64.staging:"
echo "    ssh ${RIG} 'cp -f /tmp/videonode-arm64.staging \$HOME/.local/bin/videonode && systemctl --user start videonode.service'"
