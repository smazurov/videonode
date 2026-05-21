# src/CMakeLists.txt — libraries + production binaries.
#
# Adding a new library: `vn_add_library(<name>)` (one-liner if sources are
# src/<name>.cpp; otherwise pass SOURCES). Add deps via PRIVATE_DEPS /
# PUBLIC_DEPS.
#
# Adding a new production binary: `vn_add_executable(<name> SOURCES ...
# DEPS ...)`. Pass NO_INSTALL to keep it out of the .deb/.rpm.

# ───────────────────────────────────────────────── libraries (always built)
vn_add_library(dma_heap      SOURCES dma_heap.cpp)
vn_add_library(child_process SOURCES child_process.cpp)
vn_add_library(jsonrpc_msg   SOURCES jsonrpc_msg.cpp)
vn_add_library(control_channel
    SOURCES control_channel.cpp
    PUBLIC_DEPS jsonrpc_msg)
vn_add_library(dmabuf_msg
    SOURCES dmabuf_msg.cpp
    PUBLIC_DEPS jsonrpc_msg)

vn_add_library(scm_socket
    SOURCES scm_socket.cpp
    PUBLIC_DEPS dmabuf_msg)

vn_add_library(scm_rights_source
    SOURCES scm_rights_source.cpp
    PUBLIC_DEPS scm_socket dmabuf_msg Threads::Threads)

vn_add_library(scm_rights_producer
    SOURCES scm_rights_producer.cpp
    PUBLIC_DEPS scm_socket dmabuf_msg Threads::Threads)

vn_add_library(fake_source
    SOURCES fake_source.cpp
    PUBLIC_DEPS dma_heap)

# v4l2_capture: pure UAPI ioctl wrapper for V4L2 capture devices.
vn_add_library(v4l2_capture SOURCES v4l2_capture.cpp)

# placeholder_painter: NV12 "NO SIGNAL" painter for the sidecar's
# placeholder mode. Pure CPU, no Rockchip deps.
vn_add_library(placeholder_painter SOURCES placeholder_painter.cpp)

# source_probe: event-driven source-health state machine. Pure logic.
vn_add_library(source_probe
    SOURCES source_probe.cpp
    PUBLIC_DEPS v4l2_capture)

# nv12_buf: backend-agnostic NV12 output buffer abstraction used by
# videonode-source. dma_heap single-buffer when HAVE_RGA (rig path); GBM
# split-buffer when HAVE_GBM and not HAVE_RGA (Fedora / Mesa hosts).
vn_add_library(nv12_buf
    SOURCES nv12_buf.cpp
    PUBLIC_DEPS dma_heap)
if(HAVE_RGA)
    target_compile_definitions(nv12_buf PRIVATE HAVE_RGA=1)
endif()
if(HAVE_GBM AND NOT HAVE_RGA)
    target_compile_definitions(nv12_buf PRIVATE HAVE_GBM=1)
    # gbm_alloc target is added below in the HAVE_GBM block.
endif()

# csc: backend-agnostic dispatcher exposed to videonode-source. Always
# built. With HAVE_RGA on the rig it forwards to rga_csc; on a Fedora /
# Mesa host with HAVE_GLES_CSC it forwards to csc_gles (two-pass MRT
# NV12 producer, validated by tools/csc-probe).
vn_add_library(csc SOURCES csc.cpp)

# rga_csc: librga imcvtcolor wrapper for dma-buf NV24/NV16/BGR3/YUYV/UYVY
# → NV12 conversion. Only built when real librga is present.
if(HAVE_RGA)
    vn_add_library(rga_csc
        SOURCES rga_csc.cpp
        PUBLIC_DEPS rga_iface)
    target_link_libraries(csc PUBLIC rga_csc)
    target_compile_definitions(csc PRIVATE HAVE_RGA=1)
endif()

# csc_gles: GLES2 two-pass NV24→NV12 backend. Validated by
# tools/csc-probe. NV16/YUYV/UYVY/BGR3 are TODO follow-ups.
if(HAVE_GLES_CSC)
    vn_add_library(csc_gles
        SOURCES csc_gles.cpp
        PUBLIC_DEPS egl_ctx gles_bundle)
    target_link_libraries(csc PUBLIC csc_gles)
    target_compile_definitions(csc PRIVATE HAVE_GLES_CSC=1)
endif()

# jpeg_dec_turbo: libjpeg-turbo (TurboJPEG API) software MJPEG → NV12
# decoder. Software fallback for videonode-source when MPP HW decode is
# unavailable (host builds). Real frames on x86_64, no Rockchip deps.
vn_add_library(jpeg_dec_turbo
    SOURCES jpeg_dec_turbo.cpp
    PUBLIC_DEPS PkgConfig::TURBOJPEG)

# ───────────────────────────────────────────────── GLES libs (need GBM)
if(HAVE_GBM)
    vn_add_library(gbm_alloc
        SOURCES gbm_alloc.cpp
        PUBLIC_DEPS gles_bundle)
    if(NOT HAVE_RGA)
        target_link_libraries(nv12_buf PUBLIC gbm_alloc)
    endif()

    vn_add_library(egl_ctx
        SOURCES egl_ctx.cpp
        PUBLIC_DEPS gles_bundle)

    vn_add_library(format_dispatch
        SOURCES format_dispatch.cpp
        PUBLIC_DEPS egl_ctx)

    vn_add_library(gl_compose
        SOURCES gl_compose.cpp
        PUBLIC_DEPS egl_ctx gles_bundle)
endif()

# ───────────────────────────────────────────────── Rockchip-specific libs
if(HAVE_MPP)
    vn_add_library(mpp_jpeg_dec
        SOURCES mpp_jpeg_dec.cpp
        PUBLIC_DEPS mpp_iface)
endif()

# ───────────────────────────────────────────────── production binaries
# Each gets installed into ${CMAKE_INSTALL_BINDIR} by default. Pass
# NO_INSTALL to opt out (used for none of the three production binaries).

# videonode-source — V4L2 capture sidecar. Reads frames from a V4L2 device,
# converts them to NV12 (RGA or GLES CSC for raw formats; MPP-HW or
# TurboJPEG SW decode for MJPEG), fans out dma-buf fds via SCM_RIGHTS to
# N consumers. nv12_buf hides the output allocator (dma_heap on rig, GBM
# on Fedora).
vn_add_executable(videonode-source
    SOURCES videonode_source_main.cpp
    DEPS    v4l2_capture csc nv12_buf placeholder_painter source_probe
            scm_rights_producer dmabuf_msg dma_heap
            jpeg_dec_turbo
            jsonrpc_msg control_channel
            Threads::Threads)
if(HAVE_RGA)
    target_compile_definitions(videonode-source PRIVATE HAVE_RGA=1)
endif()
if(HAVE_MPP)
    target_link_libraries(videonode-source PRIVATE mpp_jpeg_dec)
    target_compile_definitions(videonode-source PRIVATE HAVE_MPP=1)
endif()
if(HAVE_GBM AND NOT HAVE_RGA)
    # nv12_buf::Allocator::init needs a gbm_device on this backend;
    # videonode-source opens it via EglCtx at startup.
    target_compile_definitions(videonode-source PRIVATE HAVE_GBM=1)
    target_link_libraries(videonode-source PRIVATE egl_ctx gles_bundle)
endif()

# videonode-sink — single-stream NV12 carrier. Dials a videonode-source SCM socket,
# mmaps each incoming dma-buf and writes the NV12 bytes to stdout. Pipe
# into ffmpeg for transcoding to RTSP.
vn_add_executable(videonode-sink
    SOURCES videonode_sink_main.cpp
    DEPS    scm_rights_source)

# videonode-composer — Mali-GPU canvas compositor. Reads NV12 dma-bufs
# from N videonode-source sockets, composes BGRA via EGL/GLES, writes the
# canvas frames to stdout. Needs libgbm.
if(HAVE_GBM)
    vn_add_library(ffmpeg_pipe_source
        SOURCES ffmpeg_pipe_source.cpp
        PUBLIC_DEPS gbm_alloc dma_heap
        PRIVATE_DEPS child_process Threads::Threads)
    vn_add_executable(videonode-composer
        SOURCES main.cpp
        DEPS    dma_heap ffmpeg_pipe_source scm_rights_source
                format_dispatch egl_ctx gl_compose gles_bundle)
endif()
