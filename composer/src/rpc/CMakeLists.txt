# src/rpc/CMakeLists.txt — JSON-RPC framing + dma-buf envelopes + control channel.

vn_add_library(jsonrpc_msg   SOURCES jsonrpc_parse.cpp jsonrpc_serialize.cpp)

vn_add_library(dmabuf_msg
    SOURCES dmabuf_msg.cpp
    PUBLIC_DEPS jsonrpc_msg)

vn_add_library(control_channel
    SOURCES control_channel.cpp
    PUBLIC_DEPS jsonrpc_msg)
