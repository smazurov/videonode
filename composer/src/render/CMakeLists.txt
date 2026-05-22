# src/render/CMakeLists.txt — GPU compose + colorspace conversion + NV12 allocator.

vn_add_library(fake_source
    SOURCES fake_source.cpp
    PUBLIC_DEPS dma_heap)

# placeholder_painter: NV12 "NO SIGNAL" painter for the sidecar's
# placeholder mode. Pure CPU, no Rockchip deps.
vn_add_library(placeholder_painter SOURCES placeholder_painter.cpp)

# homography: corner→3×3 row-major homography solver for the GPU composer.
# Pure math, no GL, no allocation. Used by canvas_loop to translate
# daemon-supplied corner points into the per-source warp matrix.
vn_add_library(homography SOURCES homography.cpp)

# world: composer-side mirror of daemon-pushed runtime state (canvas
# dims, slot bindings, layout, effects, per-source state). Mutex-
# protected; canvas_loop snapshots per frame.
vn_add_library(world
    SOURCES world.cpp
    PUBLIC_DEPS composer_rpc homography)

# composer_service: gRPC service impl for videonode.control.Composer.
# Translates proto requests into composer_rpc structs and forwards to
# World::apply_*. Daemon dials in over the per-instance UDS.
vn_add_library(composer_service
    SOURCES composer_service.cpp
    PUBLIC_DEPS world vn_proto grpc_server)

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
endif()

# csc: backend-agnostic dispatcher exposed to videonode-source. Always
# built. Forwards to rga_csc on rig (HAVE_RGA) or csc_gles on host
# (HAVE_GLES_CSC, two-pass MRT NV12 producer).
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

# csc_gles: GLES2 two-pass NV24→NV12 backend. Validated by tools/csc-probe.
if(HAVE_GLES_CSC)
    vn_add_library(csc_gles
        SOURCES csc_gles.cpp
        PUBLIC_DEPS egl_ctx gles_bundle)
    target_link_libraries(csc PUBLIC csc_gles)
    target_compile_definitions(csc PRIVATE HAVE_GLES_CSC=1)
endif()

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

    # canvas_loop: composer compose-render-stdout loop, extracted from
    # videonode_composer_main.cpp so the binary stays argv-only.
    # Daemon-driven: reads state from `world`, services daemon RPCs via
    # `control_channel`. Owns ScmRightsSource per slot internally.
    vn_add_library(canvas_loop
        SOURCES canvas_loop.cpp
        PUBLIC_DEPS gl_compose egl_ctx scm_rights_source world control_channel gbm_alloc)
endif()
