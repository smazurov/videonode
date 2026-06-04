# src/rpc/CMakeLists.txt — gRPC server lifecycle + composer struct types
# the daemon-driven control plane uses. (Per-binary service implementations
# live in their owning libs: source/source_service, render/composer_service.)

# composer_rpc: header-only strong-typed request structs consumed by
# render::World::apply_*. Originally JSON-RPC params parsers; after the
# gRPC migration only the struct definitions remain.
add_library(composer_rpc INTERFACE)
target_sources(composer_rpc INTERFACE composer_rpc.hpp)

# grpc_server: RAII wrapper around grpc::Server bound to a Unix socket.
# Used by both videonode-source and videonode-composer to host their
# respective gRPC services on the per-instance UDS the daemon allocates.
# Public dep on grpc_bundle so the header can declare grpc::Server* /
# grpc::Service* without consumers having to find grpc++ themselves.
vn_add_library(grpc_server
    SOURCES grpc_server.cpp
    PUBLIC_DEPS grpc_bundle)
