#!/usr/bin/env bash
# install-deb.sh — fetch the latest videonode-native DEB (arm64) from
# GitHub releases and install it to /usr/bin (DEB doesn't relocate). Also
# writes a systemd-user env override pointing the Go daemon's native
# pipeline at /usr/bin/videonode-* (defaults are ~/.local/bin/videonode-*).
#
# Override VN_REPO / TAG like install-rpm.sh. Override SUDO if you want a
# non-default elevation tool.
set -euo pipefail

VN_REPO="${VN_REPO:-smazurov/videonode}"
TAG="${TAG:-}"
SUDO="${SUDO:-sudo}"
ARCH="$(uname -m)"

if [[ "$ARCH" != "aarch64" && "$ARCH" != "arm64" ]]; then
    echo "install-deb.sh: only arm64 DEB is published; got $ARCH" >&2
    exit 1
fi
if ! command -v dpkg >/dev/null; then
    echo "install-deb.sh: dpkg not found on PATH (need Debian / Ubuntu / Armbian)" >&2
    exit 1
fi

api="https://api.github.com/repos/${VN_REPO}/releases"
if [[ -n "$TAG" ]]; then
    api="${api}/tags/${TAG}"
else
    api="${api}/latest"
fi

echo ">>> querying $api"
url="$(curl -fsSL "$api" |
    grep -oE '"browser_download_url":[[:space:]]*"[^"]+_arm64\.deb"' |
    head -1 |
    sed -E 's/.*"(.+)"/\1/')"
if [[ -z "$url" ]]; then
    echo "install-deb.sh: no arm64 DEB asset found on that release" >&2
    exit 1
fi
echo ">>> downloading $url"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
curl -fsSL -o "$tmp/pkg.deb" "$url"

echo ">>> installing via $SUDO dpkg -i (target: /usr/bin)"
$SUDO dpkg -i "$tmp/pkg.deb"

echo ">>> writing systemd-user env override so the Go daemon picks /usr/bin paths"
mkdir -p "$HOME/.config/systemd/user/videonode.service.d"
cat > "$HOME/.config/systemd/user/videonode.service.d/native-pipeline.conf" <<'EOF'
[Service]
Environment="NATIVE_PIPELINE_SOURCE=/usr/bin/videonode-source"
Environment="NATIVE_PIPELINE_SINK=/usr/bin/videonode-sink"
Environment="NATIVE_PIPELINE_COMPOSER=/usr/bin/videonode-composer"
EOF
echo ">>> drop-in: $HOME/.config/systemd/user/videonode.service.d/native-pipeline.conf"

echo ">>> installed:"
for b in videonode-source videonode-sink videonode-composer; do
    p="/usr/bin/$b"
    if [[ -x "$p" ]]; then
        printf "    %s  %s\n" "$p" "$("$p" --version 2>/dev/null || echo '(--version failed)')"
    else
        printf "    %s  MISSING\n" "$p"
    fi
done

echo ">>> if videonode is running under systemd-user, reload + restart:"
echo "    systemctl --user daemon-reload && systemctl --user restart videonode"
