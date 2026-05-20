# tests/CMakeLists.txt — host-runnable unit tests.
#
# Adding a test: `vn_add_test(<short_name> DEPS ...)`. Sources default to
# tests/test_<short_name>.cpp. Each test is registered with ctest as
# `<short_name>`.

vn_add_test(ffmpeg_pipe_source_argv
    DEPS ffmpeg_pipe_source dma_heap child_process Threads::Threads)

vn_add_test(child_process       DEPS child_process)
vn_add_test(dmabuf_msg          DEPS dmabuf_msg)
vn_add_test(scm_socket          DEPS scm_socket)

vn_add_test(scm_rights_source
    DEPS scm_rights_source scm_socket dmabuf_msg Threads::Threads)

vn_add_test(scm_rights_producer
    DEPS scm_rights_producer scm_rights_source scm_socket dmabuf_msg Threads::Threads)

vn_add_test(placeholder_painter DEPS placeholder_painter)
vn_add_test(source_probe        DEPS source_probe)

if(HAVE_GBM)
    vn_add_test(format_dispatch DEPS format_dispatch)
endif()
