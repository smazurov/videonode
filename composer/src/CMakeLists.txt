# src/CMakeLists.txt — libraries + production binaries.
#
# Adding a new library: `vn_add_library(<name>)` (one-liner if sources are
# src/<name>.cpp; otherwise pass SOURCES). Add deps via PRIVATE_DEPS /
# PUBLIC_DEPS.
#
# Adding a new production binary: `vn_add_executable(<name> SOURCES ...
# DEPS ...)`. Pass STUBS if it touches the rockchip libs; pass NO_INSTALL
# if it shouldn't end up in the .deb/.rpm.

# ───────────────────────────────────────────────── libraries (always built)
vn_add_library(dma_heap      SOURCES dma_heap.cpp)
vn_add_library(child_process SOURCES child_process.cpp)
vn_add_library(jsonrpc_msg   SOURCES jsonrpc_msg.cpp)
vn_add_library(dmabuf_msg    SOURCES dmabuf_msg.cpp)

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

vn_add_library(ffmpeg_pipe_source
    SOURCES ffmpeg_pipe_source.cpp
    PUBLIC_DEPS dma_heap
    PRIVATE_DEPS child_process Threads::Threads)

# v4l2_capture: pure UAPI ioctl wrapper for V4L2 capture devices.
vn_add_library(v4l2_capture SOURCES v4l2_capture.cpp)

# placeholder_painter: NV12 "NO SIGNAL" painter for the sidecar's
# placeholder mode. Pure CPU, no Rockchip deps.
vn_add_library(placeholder_painter SOURCES placeholder_painter.cpp)

# source_probe: event-driven source-health state machine. Pure logic.
vn_add_library(source_probe
    SOURCES source_probe.cpp
    PUBLIC_DEPS v4l2_capture)

# rga_csc: librga imcvtcolor wrapper for dma-buf NV24/NV16/BGR3/YUYV/UYVY
# → NV12 conversion. Links real librga on rig; rockchip-stubs on host.
vn_add_library(rga_csc
    SOURCES rga_csc.cpp
    PUBLIC_DEPS rga_iface)

# jpeg_dec_turbo: libjpeg-turbo (TurboJPEG API) software MJPEG → NV12
# decoder. Software fallback for videonode-source when MPP HW decode is
# unavailable (host builds). Real frames on x86_64, no Rockchip deps.
vn_add_library(jpeg_dec_turbo
    SOURCES jpeg_dec_turbo.cpp
    PUBLIC_DEPS PkgConfig::TURBOJPEG)

# ───────────────────────────────────────────────── GLES libs (need GBM)
if(HAVE_GBM)
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
# converts them to NV12 (RGA CSC for raw formats, MPP-HW or TurboJPEG SW
# decode for MJPEG), fans out dma-buf fds via SCM_RIGHTS to N consumers.
vn_add_executable(videonode-source
    SOURCES videonode_source_main.cpp
    DEPS    v4l2_capture rga_csc placeholder_painter source_probe
            scm_rights_producer dmabuf_msg dma_heap
            jpeg_dec_turbo mpp_jpeg_dec
            Threads::Threads
    STUBS)

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
    vn_add_executable(videonode-composer
        SOURCES main.cpp
        DEPS    dma_heap ffmpeg_pipe_source scm_rights_source
                format_dispatch egl_ctx gl_compose gles_bundle)
endif()
