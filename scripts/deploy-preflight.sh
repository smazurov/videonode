#!/usr/bin/env bash
# Deploy preflight: prove the rig is reachable and report what is currently
# installed there, versus local HEAD. One ssh round-trip, no live-stream
# probing. Emits machine-stable key=value lines plus a single `decision:` line
# so the caller can skip a redundant deploy when the rig already runs HEAD.
#
# Staleness is decided PER COMPONENT by whether that component's source tree
# actually changed between the installed commit and HEAD — NOT by whole-repo
# version-string equality. The installed version (`videonode version` /
# `videonode-composer --version`) is a `git describe` tag whose trailing
# `-g<sha>` resolves to the commit the binary was built from; we diff that
# commit..HEAD over the component's paths. This is the whole point: a Go-only
# change leaves the C++ bins genuinely up-to-date (composer/ did not change),
# so the preflight says so instead of nagging a wasteful composer rebuild just
# to re-stamp version.hpp. It lets SKIP_COMPOSER / SKIP_GO scoping converge.
#
# Usage:
#   bash scripts/deploy-preflight.sh
#   RIG=orangepi@host bash scripts/deploy-preflight.sh
#
# Output keys (one per line): local, service, go, cpp
# Final line: `decision: UP-TO-DATE` | `decision: DEPLOY-NEEDED (<reasons>)`
#   reasons per component: no-version | unknown-commit:<ref> | src-changed | worktree-dirty
# Exit: 0 on success (either decision), non-zero only if the rig is unreachable.
set -euo pipefail

RIG="${RIG:-orangepi}"

local_ver="$(git describe --tags --always --dirty 2>/dev/null || echo dev)"

# Parse on the rig so we make exactly one connection. `videonode version`
# prints "videonode <ver> (...)"; `videonode-composer --version` prints
# "videonode-composer <ver>". Field 2 is the version token in both. A missing
# or pre-ldflags binary yields "dev"/"absent", which correctly forces a deploy.
# BatchMode + a short ConnectTimeout so an unreachable or auth-stalled rig
# fails fast and loud instead of hanging the whole deploy.
remote="$(ssh -o BatchMode=yes -o ConnectTimeout=10 "${RIG}" '
  svc=$(systemctl is-active videonode.service 2>/dev/null || echo unknown)
  go=$(/usr/bin/videonode version 2>/dev/null | head -1 | awk "{print \$2}")
  cpp=$(/usr/bin/videonode-composer --version 2>/dev/null | head -1 | awk "{print \$2}")
  echo "service=${svc}"
  echo "go=${go:-absent}"
  echo "cpp=${cpp:-absent}"
')" || { echo "FAIL: rig ${RIG} unreachable (ssh failed)" >&2; exit 1; }

go_ver="$(printf '%s\n' "${remote}" | sed -n 's/^go=//p')"
cpp_ver="$(printf '%s\n' "${remote}" | sed -n 's/^cpp=//p')"

echo "local=${local_ver}"
printf '%s\n' "${remote}"

if [ "${go_ver}" = "dev" ] || [ "${go_ver}" = "absent" ]; then
  echo "note: installed go binary carries no real version — build-go-arm64.sh must inject the version ldflags"
fi

# Echoes a staleness reason for a component, or empty if it is up to date.
# $1 = installed version string; $2.. = git pathspecs that affect the component.
component_stale() {
  local ver="$1"; shift
  case "${ver}" in
    dev|unknown|absent|"") echo "no-version"; return ;;
  esac
  # `git describe` tags look like v1.2.3-4-g<sha>[-dirty]; the build commit is
  # the sha after the last `-g`. Strip a `-dirty` suffix first, then take the
  # sha. A clean exact tag (no `-g`) resolves on its own.
  local ref="${ver%-dirty}"
  case "${ref}" in *-g*) ref="${ref##*-g}" ;; esac
  if ! git rev-parse --verify --quiet "${ref}^{commit}" >/dev/null 2>&1; then
    echo "unknown-commit:${ref}"; return
  fi
  if ! git diff --quiet "${ref}..HEAD" -- "$@" 2>/dev/null; then
    echo "src-changed"; return
  fi
  if [ -n "$(git status --porcelain -- "$@" 2>/dev/null)" ]; then
    echo "worktree-dirty"; return
  fi
  echo ""
}

# Go binary tracks Go source + the embedded UI; C++ helpers track composer/
# (and proto/, which protoc consumes at composer build time). Checked-in
# generated proto stubs under internal/ are .go files, so the go glob covers them.
go_stale="$(component_stale "${go_ver}"  '*.go' go.mod go.sum ui/)"
cpp_stale="$(component_stale "${cpp_ver}" composer/ proto/)"

diffs=()
[ -n "${go_stale}" ]  && diffs+=("go:${go_stale}")
[ -n "${cpp_stale}" ] && diffs+=("cpp:${cpp_stale}")

if [ ${#diffs[@]} -eq 0 ]; then
  echo "decision: UP-TO-DATE"
else
  echo "decision: DEPLOY-NEEDED (${diffs[*]})"
fi
