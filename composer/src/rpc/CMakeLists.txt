# src/rpc/CMakeLists.txt — JSON-RPC framing + dma-buf envelopes + control channel.

vn_add_library(jsonrpc_msg   SOURCES jsonrpc_parse.cpp jsonrpc_serialize.cpp)

vn_add_library(dmabuf_msg
    SOURCES dmabuf_msg.cpp
    PUBLIC_DEPS jsonrpc_msg)

vn_add_library(control_channel
    SOURCES control_channel.cpp
    PUBLIC_DEPS jsonrpc_msg)

# composer_rpc: params parsers for daemon→composer JSON-RPC methods
# (set_canvas / set_source / clear_source / set_layout / set_effects /
# set_source_state). Untyped wire bytes in, strong-typed structs out.
vn_add_library(composer_rpc
    SOURCES composer_rpc.cpp
    PUBLIC_DEPS jsonrpc_msg)

# grpc_server: RAII wrapper around grpc::Server bound to a Unix socket.
# Used by both videonode-source and videonode-composer to host their
# respective gRPC services on the per-instance UDS the daemon allocates.
# Public dep on grpc_bundle so the header can declare grpc::Server* /
# grpc::Service* without consumers having to find grpc++ themselves.
vn_add_library(grpc_server
    SOURCES grpc_server.cpp
    PUBLIC_DEPS grpc_bundle)
