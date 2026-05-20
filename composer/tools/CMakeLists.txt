# tools/CMakeLists.txt — diagnostic probes. Not installed (developer-only).
#
# Adding a probe: `vn_add_probe(<name> DEPS ...)`. If the source isn't at
# tools/<name>.cpp, pass SOURCES.

vn_add_probe(dma-probe         DEPS dma_heap)
vn_add_probe(source-probe      DEPS dma_heap fake_source)
vn_add_probe(pipe-source-probe DEPS dma_heap ffmpeg_pipe_source)

if(HAVE_GBM)
    vn_add_probe(egl-probe             DEPS gles_bundle)
    vn_add_probe(import-probe          DEPS dma_heap fake_source egl_ctx gles_bundle)
    vn_add_probe(compose-probe         DEPS dma_heap fake_source egl_ctx gl_compose gles_bundle)
    vn_add_probe(live-compose-probe    DEPS dma_heap ffmpeg_pipe_source egl_ctx gl_compose gles_bundle)
    vn_add_probe(dmabuf-format-probe   DEPS egl_ctx gles_bundle)
endif()

# rga-csc-probe runs on the rig only (HAVE_RGA real, not stubbed). On host
# the stubs return no-op success which would mask the kind of bugs this
# probe catches.
if(HAVE_RGA AND NOT USING_ROCKCHIP_STUBS)
    vn_add_probe(rga-csc-probe DEPS rga_csc dma_heap)
    if(BUILD_TESTS)
        add_test(NAME rga_csc_probe COMMAND rga-csc-probe)
    endif()
endif()
