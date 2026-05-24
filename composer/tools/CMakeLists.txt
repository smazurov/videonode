# tools/CMakeLists.txt — diagnostic probes. Not installed (developer-only).
#
# Adding a probe: `vn_add_probe(<name> DEPS ...)`. If the source isn't at
# tools/<name>.cpp, pass SOURCES.

vn_add_probe(dma-probe         DEPS dma_heap)
vn_add_probe(source-probe      DEPS dma_heap fake_source)

if(HAVE_GBM)
    vn_add_probe(egl-probe             DEPS egl_ctx gles_bundle)
    vn_add_probe(import-probe          DEPS dma_heap fake_source egl_ctx gles_bundle)
    vn_add_probe(pipe-source-probe     DEPS dma_heap ffmpeg_pipe_source)
    vn_add_probe(dmabuf-format-probe   DEPS egl_ctx gles_bundle)
endif()

if(HAVE_RGA)
    vn_add_probe(rga-csc-probe DEPS rga_csc dma_heap)
    if(BUILD_TESTS)
        add_test(NAME rga_csc_probe COMMAND rga-csc-probe)
    endif()
endif()

# libplacebo evaluation probes (issue #9)
if(HAVE_PLACEBO AND HAVE_GBM)
    vn_add_probe(placebo-csc-probe DEPS egl_ctx gles_bundle placebo_bundle)
    if(HAVE_VULKAN)
        vn_add_probe(placebo-vk-probe         DEPS placebo_bundle PkgConfig::GBM PkgConfig::DRM)
        vn_add_probe(placebo-host-copy-probe  DEPS placebo_bundle PkgConfig::GBM PkgConfig::DRM)
    endif()
endif()
