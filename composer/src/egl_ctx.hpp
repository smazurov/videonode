// egl_ctx — minimal EGL/GBM/GLES2 context for headless dma-buf rendering.
//
// Wraps the moves egl-probe established: open a DRM render node, create a
// GBM device, initialize an EGL display via EGL_PLATFORM_GBM_KHR, create a
// surfaceless GLES2 context using EGL_KHR_no_config_context, and provide a
// helper that turns a dma-buf fd (NV12 from fake_source, RGBA from a GBM bo,
// etc.) into an EGLImage we can bind to a texture or renderbuffer.
//
// What this gives the composer: one class to own GPU connection lifetime
// (open/close DRM fd, make-current, teardown) and one entry point for
// importing external dma-bufs (the synthetic sources, eventually V4L2 sources).
//
// Why a class instead of free functions: the gbm_device + EGLDisplay + EGLContext
// are tied. Making them members keeps destruction ordering tidy and lets the
// compose code take an EglCtx& rather than four handles.

#pragma once

#include <EGL/egl.h>
#include <EGL/eglext.h>

#include <cstdint>
#include <string_view>

struct gbm_device;

namespace egl_ctx {

class EglCtx {
  public:
    // Open the render node, init EGL, bind a surfaceless GLES2 context.
    // Returns false on any failure; check before use.
    bool init(std::string_view device_path = "/dev/dri/renderD130");
    ~EglCtx();

    EglCtx() = default;
    EglCtx(const EglCtx&) = delete;
    EglCtx& operator=(const EglCtx&) = delete;

    EGLDisplay display() const { return dpy_; }
    EGLContext context() const { return ctx_; }
    gbm_device* gbm() const { return gbm_; }
    int drm_fd() const { return drm_fd_; }

    // Convenience: bind the context to the calling thread (no surfaces).
    bool make_current() const;

    // Build an EGLImage from a dma-buf fd. Supports two layouts the spike
    // uses today:
    //   - NV12 (two planes; plane 1 is interleaved CbCr at half resolution).
    //     Pass fourcc = DRM_FORMAT_NV12, the same fd for both planes,
    //     plane0_pitch = width, plane1_pitch = width, plane1_offset = w*h.
    //   - Single-plane RGB/BGRA. Pass fourcc + plane0_* only and leave the
    //     plane1_* zeros.
    // Modifier = DRM_FORMAT_MOD_LINEAR for sources we own (dma_heap-backed).
    // GBM-allocated bo's may carry their own modifier — query gbm_bo_get_modifier
    // and pass that explicitly.
    struct ImageDesc {
        int fd = -1;           // dma-buf fd (caller retains ownership)
        uint32_t fourcc = 0;   // DRM_FORMAT_*
        uint64_t modifier = 0; // DRM_FORMAT_MOD_*
        int width = 0;
        int height = 0;
        int plane0_offset = 0;
        int plane0_pitch = 0;
        int plane1_offset = 0; // for NV12; 0 for single-plane
        int plane1_pitch = 0;  // for NV12; 0 for single-plane
    };
    EGLImage import_dmabuf(const ImageDesc& d) const;

  private:
    int drm_fd_ = -1;
    gbm_device* gbm_ = nullptr;
    EGLDisplay dpy_ = EGL_NO_DISPLAY;
    EGLContext ctx_ = EGL_NO_CONTEXT;
};

} // namespace egl_ctx
