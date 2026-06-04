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
    [[nodiscard]] bool init(std::string_view device_path = "/dev/dri/renderD130");
    ~EglCtx();

    EglCtx() = default;
    EglCtx(const EglCtx&) = delete;
    EglCtx& operator=(const EglCtx&) = delete;

    [[nodiscard]] EGLDisplay display() const { return dpy_; }
    [[nodiscard]] EGLContext context() const { return ctx_; }
    [[nodiscard]] gbm_device* gbm() const { return gbm_; }
    [[nodiscard]] int drm_fd() const { return drm_fd_; }

    [[nodiscard]] bool make_current() const;

    // Build an EGLImage from a dma-buf fd. Supports two layouts the composer
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
        int fd = -1;        // dma-buf fd for plane 0 (caller retains ownership)
        int plane1_fd = -1; // optional separate fd for plane 1; -1 → reuse fd
        uint32_t fourcc = 0;
        uint64_t modifier = 0;
        int width = 0;
        int height = 0;
        int plane0_offset = 0;
        int plane0_pitch = 0;
        int plane1_offset = 0; // for NV12; 0 for single-plane
        int plane1_pitch = 0;  // for NV12; 0 for single-plane
    };
    [[nodiscard]] EGLImage import_dmabuf(const ImageDesc& d) const;

  private:
    int drm_fd_ = -1;
    gbm_device* gbm_ = nullptr;
    EGLDisplay dpy_ = EGL_NO_DISPLAY;
    EGLContext ctx_ = EGL_NO_CONTEXT;
};

} // namespace egl_ctx
