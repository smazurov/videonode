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
vn_add_test(jsonrpc_msg         DEPS jsonrpc_msg                  LABELS rpc)
vn_add_test(dmabuf_msg          DEPS dmabuf_msg                   LABELS rpc)
vn_add_test(scm_socket          DEPS scm_socket                   LABELS ipc)

vn_add_test(scm_rights_source
    DEPS scm_rights_source scm_socket dmabuf_msg Threads::Threads
    LABELS ipc)

vn_add_test(scm_rights_producer
    DEPS scm_rights_producer scm_rights_source scm_socket dmabuf_msg Threads::Threads
    LABELS ipc)

vn_add_test(placeholder_painter DEPS placeholder_painter          LABELS render)
vn_add_test(source_probe        DEPS source_probe                 LABELS capture)

if(HAVE_GBM)
    vn_add_test(format_dispatch DEPS format_dispatch              LABELS render)
endif()
