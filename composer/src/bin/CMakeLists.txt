# src/bin/CMakeLists.txt — production binaries.
#
# Each gets installed into ${CMAKE_INSTALL_BINDIR} by default. Pass
# NO_INSTALL to opt out (used for none of the three production binaries).

# videonode-source — V4L2 capture sidecar. Reads frames from a V4L2 device,
# converts them to NV12 (RGA or GLES CSC for raw formats; MPP-HW or
# TurboJPEG SW decode for MJPEG), fans out dma-buf fds via SCM_RIGHTS to
# N consumers. nv12_buf hides the output allocator (dma_heap on rig, GBM
# on Fedora).
vn_add_executable(videonode-source
    SOURCES videonode_source_main.cpp
    DEPS    orchestrator absl_bundle)

# videonode-sink — single-stream NV12 carrier. Dials a videonode-source SCM
# socket, mmaps each incoming dma-buf and writes the NV12 bytes to stdout.
# Pipe into ffmpeg for transcoding to RTSP.
vn_add_executable(videonode-sink
    SOURCES videonode_sink_main.cpp
    DEPS    scm_rights_source absl_bundle)
if(CMAKE_SYSTEM_PROCESSOR MATCHES "x86_64|AMD64")
    target_compile_options(videonode-sink PRIVATE -msse4.1)
endif()

# videonode-composer — Mali-GPU canvas compositor. Reads NV12 dma-bufs
# from N videonode-source sockets, composes BGRA via EGL/GLES, writes the
# canvas frames to stdout. Needs libgbm.
if(HAVE_PLACEBO AND HAVE_GBM)
    vn_add_executable(videonode-composer
        SOURCES videonode_composer_main.cpp
        DEPS    canvas_loop grpc_server absl_bundle)
endif()