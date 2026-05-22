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
