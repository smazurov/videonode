# tests/CMakeLists.txt — host-runnable unit tests.
#
# Adding a test: `vn_add_test(<short_name> DEPS ... LABELS ...)`. Sources
# default to tests/test_<short_name>.cpp. Each TEST() macro in the file is
# auto-registered with ctest via gtest_discover_tests. LABELS lets ctest -L
# select a domain (ipc, rpc, capture, render, process).

vn_add_test(ffmpeg_pipe_source_argv
    DEPS ffmpeg_pipe_source dma_heap child_process egl_ctx gles_bundle
         Threads::Threads
    LABELS process)

vn_add_test(child_process       DEPS child_process                LABELS process)
vn_add_test(snapshot
    DEPS vn_snapshot Threads::Threads
    LABELS snapshot)
vn_add_test(scm_socket          DEPS scm_socket                   LABELS ipc)
vn_add_test(dmabuf_header       DEPS dmabuf_header                LABELS ipc)

vn_add_test(scm_rights_source
    DEPS scm_rights_source scm_socket dmabuf_header Threads::Threads
    LABELS ipc)

vn_add_test(scm_rights_producer
    DEPS scm_rights_producer scm_rights_source scm_socket dmabuf_header Threads::Threads
    LABELS ipc)

vn_add_test(scm_ctrunc_repro
    DEPS scm_socket dmabuf_header Threads::Threads
    LABELS ipc)

vn_add_test(placeholder_painter DEPS placeholder_painter          LABELS render)
vn_add_test(homography          DEPS homography                   LABELS render)
vn_add_test(world               DEPS world Threads::Threads       LABELS render)
vn_add_test(source_probe        DEPS source_probe                 LABELS capture)
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
        DEPS jpeg_dec_turbo nv12_buf csc_placebo egl_ctx gles_bundle placebo_bundle PkgConfig::TURBOJPEG
        LABELS capture)
endif()
