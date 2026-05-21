#!/usr/bin/env bash
# install-rpm.sh — fetch the latest videonode-native RPM from GitHub
# releases and install it relocated to $HOME/.local (so the binaries land
# in $HOME/.local/bin/videonode-{source,sink,composer} where the Go
# daemon expects them by default).
#
# Override the GitHub repo via VN_REPO=owner/name; default = smazurov/videonode.
# Override the install prefix via PREFIX=/some/path.
# Pin a specific tag via TAG=native-v0.1.0; default = latest.
set -euo pipefail

VN_REPO="${VN_REPO:-smazurov/videonode}"
PREFIX="${PREFIX:-$HOME/.local}"
TAG="${TAG:-}"
ARCH="$(uname -m)"

if [[ "$ARCH" != "x86_64" ]]; then
    echo "install-rpm.sh: only x86_64 RPM is published; got $ARCH" >&2
    exit 1
fi
if ! command -v rpm >/dev/null; then
    echo "install-rpm.sh: rpm not found on PATH (need Fedora / RHEL / openSUSE)" >&2
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
    grep -oE '"browser_download_url":[[:space:]]*"[^"]+\.x86_64\.rpm"' |
    head -1 |
    sed -E 's/.*"(.+)"/\1/')"
if [[ -z "$url" ]]; then
    echo "install-rpm.sh: no x86_64 RPM asset found on that release" >&2
    exit 1
fi
echo ">>> downloading $url"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
curl -fsSL -o "$tmp/pkg.rpm" "$url"

echo ">>> installing to $PREFIX (relocatable RPM)"
mkdir -p "$PREFIX/bin"
rpm --install --upgrade --replacepkgs --replacefiles \
    --prefix="$PREFIX" \
    --dbpath="$HOME/.local/share/rpm-userdb" \
    "$tmp/pkg.rpm" 2>/dev/null || {
    # rpm --dbpath without root often hits perms; fall back to extracting
    # the cpio payload directly into $PREFIX. This keeps the relocatable
    # contract without touching the system rpm db.
    echo ">>> rpm db install rejected; extracting payload into $PREFIX"
    (cd "$PREFIX" && rpm2cpio "$tmp/pkg.rpm" |
        cpio -idm --quiet --no-absolute-filenames)
}

echo ">>> installed:"
for b in videonode-source videonode-sink videonode-composer; do
    p="$PREFIX/bin/$b"
    if [[ -x "$p" ]]; then
        printf "    %s  %s\n" "$p" "$("$p" --version 2>/dev/null || echo '(--version failed)')"
    else
        printf "    %s  MISSING\n" "$p"
    fi
done

case ":$PATH:" in
    *":$PREFIX/bin:"*) ;;
    *) echo ">>> note: $PREFIX/bin is not on PATH" ;;
esac
