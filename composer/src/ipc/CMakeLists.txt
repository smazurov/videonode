# src/ipc/CMakeLists.txt — Unix-domain fd passing primitives.

vn_add_library(dma_heap      SOURCES dma_heap.cpp)

# dmabuf_header: fixed-shape binary wire format for the SCM_RIGHTS data
# plane. Replaces the legacy JSON-RPC envelope (rpc/dmabuf_msg) — same
# semantic shape, ~150 LOC encoder/decoder with no JSON dependency.
vn_add_library(dmabuf_header SOURCES dmabuf_header.cpp)

vn_add_library(scm_socket
    SOURCES scm_socket.cpp
    PUBLIC_DEPS dmabuf_header)

vn_add_library(scm_rights_source
    SOURCES scm_rights_source.cpp
    PUBLIC_DEPS scm_socket dmabuf_header Threads::Threads)

vn_add_library(scm_rights_producer
    SOURCES scm_rights_producer.cpp
    PUBLIC_DEPS scm_socket dmabuf_header Threads::Threads)
