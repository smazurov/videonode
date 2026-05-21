// gl_compose — multi-source GPU compositor.
//
// Owns a canvas RGBA dma-buf (allocated via GBM, exported for downstream RGA
// CSC + encoder consumption), a GLES2 shader program, and per-source textures
// bound from external EGLImages. Each frame: clears the canvas, draws each
// source as a textured quad at its assigned canvas region, with an optional
// per-source 3x3 warp matrix applied to the sampling UVs (the perspective
// unlock — works on Mali-G610 here, would require a SW path on RGA).
//
// What this proves for the production architecture:
//   - GLES2 + samplerExternalOES + GBM canvas is enough to do composition
//     end-to-end on Mali via Panthor.
//   - Per-source perspective is a single uniform; switching it on/off per
//     source costs nothing structurally (no decode-path change).

#pragma once

#include "src/render/egl_ctx.hpp"

#include <EGL/egl.h>
#include <GLES2/gl2.h>
#include <array>
#include <cstdint>
#include <string_view>
#include <vector>

struct gbm_bo;

namespace gl_compose {

// 3x3 row-major homography. Identity = pass-through.
struct Warp {
    float m[9] = {1, 0, 0, 0, 1, 0, 0, 0, 1};
};

// One source bound to one canvas slot. Two EGLImages per source: Y as R8
// (luma), UV as GR88 (chroma at half resolution). Each is a single-plane
// import from its own dma-buf fd at PLANE0_OFFSET=0 — that's the only
// pattern radeonsi/amdgpu reliably samples (per minigbm/Chromium AMD
// path). The fragment shader does BT.601 limited YUV→RGB manually with
// two sampler2D uniforms; that side-steps `samplerExternalOES`, which
// is also broken on radeonsi for NV12 dma-buf imports.
struct SourceSlot {
    EGLImage src_y_image = EGL_NO_IMAGE;  // R8, W×H
    EGLImage src_uv_image = EGL_NO_IMAGE; // GR88, W/2×H/2
    int x = 0;                            // canvas-px placement (top-left)
    int y = 0;
    int w = 0; // slot size in canvas-px
    int h = 0;
    Warp warp; // applied to UVs
};

class GlCompose {
  public:
    // Initialize: allocate canvas (GBM_FORMAT_ARGB8888), compile shaders,
    // build the attribute buffers. Requires an already-initialized EglCtx.
    [[nodiscard]] bool init(egl_ctx::EglCtx& ctx, int canvas_w, int canvas_h);
    ~GlCompose();

    GlCompose() = default;
    GlCompose(const GlCompose&) = delete;
    GlCompose& operator=(const GlCompose&) = delete;

    // Render one canvas frame from the given slots. Returns true on success.
    // Caller is responsible for src_image lifetime; we sample but don't own.
    [[nodiscard]] bool render(const std::vector<SourceSlot>& slots);

    // Wait for the GPU to finish the last render. Use before the
    // downstream consumer (RGA / mmap) reads the canvas.
    void finish();

    // Canvas access for downstream stages.
    int canvas_dmabuf_fd() const { return canvas_fd_; }
    int canvas_w() const { return canvas_w_; }
    int canvas_h() const { return canvas_h_; }
    uint32_t canvas_stride() const { return canvas_stride_; }
    gbm_bo* canvas_bo() const { return canvas_bo_; }

  private:
    bool build_program_(std::string_view vs_src, std::string_view fs_src);
    bool make_canvas_(int w, int h);

    egl_ctx::EglCtx* ctx_ = nullptr;
    int canvas_w_ = 0;
    int canvas_h_ = 0;
    uint32_t canvas_stride_ = 0;
    gbm_bo* canvas_bo_ = nullptr;
    int canvas_fd_ = -1;
    EGLImage canvas_img_ = EGL_NO_IMAGE;
    GLuint rbo_ = 0;
    GLuint fbo_ = 0;
    GLuint prog_ = 0;
    GLuint vbo_ = 0;
    GLuint ibo_ = 0;
    GLint loc_canvas_size_ = -1;
    GLint loc_warp_ = -1;
    GLint loc_src_y_ = -1;
    GLint loc_src_uv_ = -1;
    GLint attr_pos_ = -1;
    GLint attr_uv_ = -1;
};

} // namespace gl_compose
