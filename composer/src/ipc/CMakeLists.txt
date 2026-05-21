# src/ipc/CMakeLists.txt — Unix-domain fd passing primitives.

vn_add_library(dma_heap      SOURCES dma_heap.cpp)

vn_add_library(scm_socket
    SOURCES scm_socket.cpp
    PUBLIC_DEPS dmabuf_msg)

vn_add_library(scm_rights_source
    SOURCES scm_rights_source.cpp
    PUBLIC_DEPS scm_socket dmabuf_msg Threads::Threads)

vn_add_library(scm_rights_producer
    SOURCES scm_rights_producer.cpp
    PUBLIC_DEPS scm_socket dmabuf_msg Threads::Threads)
