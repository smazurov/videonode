# tests/CMakeLists.txt — host-runnable unit tests.
#
# Two registration styles:
#
# 1. Pure logic tests (single-threaded, no sockets/fork/GL, always build)
#    live under tests/unit/. Drop a test_*.cpp there — no CMake edit needed.
#    vn_add_unit_suite globs the dir, links all UNIT_SAFE libs, and registers
#    each TEST() case as its own ctest entry via gtest_discover_tests.
#
# 2. Tests that need process isolation (crash/hang risk), special link
#    deps, or HAVE_* gating stay on `vn_add_test(<short> DEPS ... LABELS ...)`,
#    one executable each. Sources default to tests/test_<short>.cpp.

vn_add_unit_suite(vn_unit_tests GLOB "${CMAKE_CURRENT_SOURCE_DIR}/unit" LABELS unit)

vn_add_test(ffmpeg_pipe_source_argv
    DEPS ffmpeg_pipe_source dma_heap egl_ctx Threads::Threads
    LABELS process)

vn_add_test(child_process       DEPS child_process                LABELS process)
vn_add_test(snapshot
    DEPS vn_snapshot Threads::Threads
    LABELS snapshot)
vn_add_test(scm_socket          DEPS scm_socket                   LABELS ipc)

vn_add_test(scm_rights_source
    DEPS scm_rights_source
    LABELS ipc)

vn_add_test(scm_rights_producer
    DEPS scm_rights_producer scm_rights_source
    LABELS ipc)

vn_add_test(scm_ctrunc_repro
    DEPS scm_socket Threads::Threads
    LABELS ipc)

if(HAVE_RGA)
    vn_add_test(nv12_stride       DEPS nv12_buf                   LABELS render)
endif()
vn_add_test(world               DEPS world Threads::Threads       LABELS render)
vn_add_test(jpeg_dec_turbo
    DEPS jpeg_dec_turbo PkgConfig::TURBOJPEG
    LABELS capture)

if(HAVE_GBM)
    vn_add_test(format_dispatch DEPS format_dispatch              LABELS render)
endif()

# Regression for the all-green-frame bug: TurboJPEG decodes into an
# nv12_buf slot via held CPU mmap; a separate consumer mmap of the
# same dma-buf fd reads stale (zero) bytes because the source path
# never unmaps or sync_end's the slot before broadcasting.
if(HAVE_PLACEBO AND HAVE_GBM)
    vn_add_test(jpeg_dec_dmabuf_visibility
        DEPS jpeg_dec_turbo nv12_buf csc_placebo PkgConfig::TURBOJPEG
        LABELS capture)

    vn_add_test(csc_placebo_yuyv
        DEPS csc gbm_alloc
        LABELS render)

    vn_add_test(pl_compose_fd_leak
        DEPS pl_compose gbm_alloc
        LABELS render)

    vn_add_test(canvas_clear
        DEPS pl_compose gbm_alloc
        LABELS render)

    vn_add_test(canvas_tearing
        DEPS pl_compose gbm_alloc Threads::Threads
        LABELS render)

    vn_add_test(nv12_ring_overwrite
        DEPS gbm_alloc
        LABELS render)

    vn_add_test(pipeline_tearing
        DEPS pl_compose gbm_alloc nv12_buf scm_rights_producer scm_rights_source
        LABELS render)

endif()

vn_add_test(fd_lifetime
    DEPS scm_rights_producer scm_rights_source
    LABELS ipc)
