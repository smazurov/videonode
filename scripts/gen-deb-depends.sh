#!/usr/bin/env bash
# Emit the .deb runtime `Depends` list, one entry per line, from the actual ELF
# binaries — so the package's library deps (sonames + version floors) are derived
# from what the binaries really link, never a hand-maintained list that can drift.
#
# Usage: scripts/gen-deb-depends.sh <binary>...   # prints deps to stdout
#
# Must run where the binaries' linked libraries are installed WITH their dpkg
# symbols/shlibs metadata — i.e. inside the arm64 Debian trixie build container
# (the release.yml build job), not the cross x86_64 package job. dpkg-shlibdeps
# reads each DT_NEEDED soname's owning package + symbols file to compute a
# version-floored, ABI-correct dependency (e.g. `libplacebo349 (>= 7.349.0)`).
#
# Static binaries (the CGO_ENABLED=0 Go daemon) have no dynamic deps and are
# skipped. Board libs without dpkg metadata (librga / librockchip_mpp, from the
# tsukumijima .debs) are skipped via --ignore-missing-info: they stay undeclared
# because they're provided out-of-band by install-rockchip-libs.sh / the image.
set -euo pipefail

if [[ $# -eq 0 ]]; then
    echo "usage: $0 <binary>..." >&2
    exit 2
fi

if ! command -v dpkg-shlibdeps >/dev/null 2>&1; then
    echo "$0: dpkg-shlibdeps not found (install dpkg-dev)" >&2
    exit 1
fi

# Keep only dynamically-linked ELF executables (those with a PT_INTERP); a fully
# static Go binary has none and would make dpkg-shlibdeps error out.
dyn_bins=()
for bin in "$@"; do
    if [[ -f "$bin" ]] && readelf -l "$bin" 2>/dev/null | grep -q INTERP; then
        dyn_bins+=("$(readlink -f "$bin")")
    fi
done

if [[ ${#dyn_bins[@]} -eq 0 ]]; then
    echo "$0: no dynamically-linked binaries among: $*" >&2
    exit 1
fi

# dpkg-shlibdeps requires a debian/control; build a throwaway one in a tmp dir and
# use -O to print `shlibs:Depends=...` to stdout instead of writing debian/substvars.
workdir="$(mktemp -d)"
trap 'rm -rf "$workdir"' EXIT
mkdir -p "$workdir/debian"
cat > "$workdir/debian/control" <<EOF
Source: videonode
Package: videonode
Architecture: $(dpkg --print-architecture)
EOF

shlibs_line="$(cd "$workdir" && dpkg-shlibdeps -O --ignore-missing-info "${dyn_bins[@]}")"

# Output is `shlibs:Depends=dep1, dep2, ...`. Strip the prefix, split on commas.
deps="${shlibs_line#shlibs:Depends=}"

# base-files (>= 13~) is the friendly trixie OS-gate dpkg-shlibdeps won't emit:
# it turns a wrong-release install into a readable "needs Debian 13" instead of a
# pile of uninstallable-soname errors. The ~ admits any trixie point release.
echo "base-files (>= 13~)"
# sed (not grep) to drop blank lines: returns 0 even when the lib list is empty
# (e.g. a non-dpkg host), so set -e/pipefail don't kill the script on no-match.
echo "$deps" | tr ',' '\n' | sed 's/^[[:space:]]*//;s/[[:space:]]*$//;/^$/d'
