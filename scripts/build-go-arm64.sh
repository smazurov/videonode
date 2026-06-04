#!/usr/bin/env bash
# Cross-build the arm64 videonode supervisor for the rig and stage it to
# /tmp there (the canonical /usr/bin swap happens later, under sudo, with the
# service stopped — see the deploy-rig skill step 4). Deterministic so the
# skill calls it instead of reciting the pnpm/go/scp dance by hand.
#
# Always builds with `-tags ui_embed` + the version ldflags so `videonode
# version` on the rig carries a real tag (the deploy preflight depends on it).
#
# Env:
#   RIG=user@host   ssh target for staging (default: orangepi)
#   SKIP_UI=1       skip `pnpm build` (only safe if ui/dist/ is already current)
#   STAGE=0         build locally only, do not scp to the rig
#   OUT=path        local output (default: bin/videonode-arm64)
#   STAGE_PATH=path remote staging path (default: /tmp/videonode-arm64.staging)
set -euo pipefail

RIG="${RIG:-orangepi}"
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT}"
OUT="${OUT:-bin/videonode-arm64}"
STAGE_PATH="${STAGE_PATH:-/tmp/videonode-arm64.staging}"

# `pnpm --prefix ui build`, not `cd ui && pnpm build`: the latter leaves cwd in
# ui/ and the subsequent `go build .` silently emits an ar archive.
if [[ "${SKIP_UI:-0}" != "1" ]]; then
  echo ">>> ui: pnpm build"
  pnpm --prefix ui install --frozen-lockfile
  pnpm --prefix ui build
else
  echo ">>> ui: SKIP_UI=1 (reusing existing ui/dist/)"
fi

VERSION="$(git describe --tags --always --dirty 2>/dev/null || echo dev)"
echo ">>> go build linux/arm64 (-tags ui_embed, version=${VERSION})"
mkdir -p "$(dirname "${OUT}")"
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -tags ui_embed \
  -ldflags "-X 'github.com/smazurov/videonode/internal/version.Version=${VERSION}'" \
  -o "${OUT}" .
file "${OUT}"

if [[ "${STAGE:-1}" != "0" ]]; then
  echo ">>> stage to ${RIG}:${STAGE_PATH}"
  scp "${OUT}" "${RIG}:${STAGE_PATH}"
  echo "staged: ${RIG}:${STAGE_PATH}"
fi
