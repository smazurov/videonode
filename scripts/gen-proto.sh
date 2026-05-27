#!/usr/bin/env bash
# Regenerate Go protobuf + gRPC stubs from proto/control/*.proto.
#
# Tool versions are pinned via the `tool` directive in go.mod (Go 1.24+).
# We build the plugin binaries to a temp dir and put it on PATH so protoc
# can discover them — protoc plugin discovery doesn't support `go tool`
# directly.

set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

if ! command -v protoc >/dev/null 2>&1; then
  echo "[gen-proto] protoc not found in PATH" >&2
  echo "[gen-proto] install: sudo dnf install protobuf-compiler (Fedora)" >&2
  echo "[gen-proto]      or: sudo apt install protobuf-compiler   (Debian)" >&2
  exit 1
fi

BIN_DIR="$(mktemp -d)"
trap 'rm -rf "$BIN_DIR"' EXIT

go build -o "$BIN_DIR/protoc-gen-go" google.golang.org/protobuf/cmd/protoc-gen-go
go build -o "$BIN_DIR/protoc-gen-go-grpc" google.golang.org/grpc/cmd/protoc-gen-go-grpc

mkdir -p internal/streams/pipelinectl/pb

PATH="$BIN_DIR:$PATH" protoc \
  -I proto \
  --go_out="$ROOT" --go_opt=module=github.com/smazurov/videonode \
  --go-grpc_out="$ROOT" --go-grpc_opt=module=github.com/smazurov/videonode \
  proto/control/common.proto \
  proto/control/source.proto \
  proto/control/composer.proto

echo "[gen-proto] wrote internal/streams/pipelinectl/pb/*.pb.go"
