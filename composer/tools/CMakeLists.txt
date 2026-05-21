# tools/CMakeLists.txt — diagnostic probes. Not installed (developer-only).
#
# Adding a probe: `vn_add_probe(<name> DEPS ...)`. If the source isn't at
# tools/<name>.cpp, pass SOURCES.

vn_add_probe(dma-probe         DEPS dma_heap)
vn_add_probe(source-probe      DEPS dma_heap fake_source)

if(HAVE_GBM)
    vn_add_probe(egl-probe             DEPS gles_bundle)
    vn_add_probe(import-probe          DEPS dma_heap fake_source egl_ctx gles_bundle)
    # compose-probe and live-compose-probe use the old single-EGLImage NV12
    # gl_compose API; they're temporarily disabled while gl_compose runs the
    # two-fd R8/GR88 path on AMD. To revisit once the API settles.
    # vn_add_probe(compose-probe         DEPS dma_heap fake_source egl_ctx gl_compose gles_bundle)
    vn_add_probe(pipe-source-probe     DEPS dma_heap ffmpeg_pipe_source)
    # vn_add_probe(live-compose-probe    DEPS dma_heap ffmpeg_pipe_source egl_ctx gl_compose gles_bundle)
    vn_add_probe(dmabuf-format-probe   DEPS egl_ctx gles_bundle)
    vn_add_probe(csc-probe             DEPS egl_ctx gles_bundle)
endif()

if(HAVE_RGA)
    vn_add_probe(rga-csc-probe DEPS rga_csc dma_heap)
    if(BUILD_TESTS)
        add_test(NAME rga_csc_probe COMMAND rga-csc-probe)
    endif()
endif()
