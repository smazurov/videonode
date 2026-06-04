//go:build generate

// This file exists to host the project-wide `go generate` directive that
// regenerates Go protobuf + gRPC stubs from proto/control/*.proto. The
// build tag keeps it out of the regular build; `go generate ./...` still
// picks it up.

package main

//go:generate ./scripts/gen-proto.sh
