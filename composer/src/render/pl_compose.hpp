// pl_compose — libplacebo multi-source GPU compositor.
//
// Replaces gl_compose. Owns a BGRA canvas dma-buf, imports per-source
// NV12 dma-bufs as pl_tex, composites via pl_dispatch with an optional
// per-source 3×3 homography warp. Backend-agnostic (OpenGL today,
// Vulkan later).

#pragma once

#include "src/common/owner.hpp"

#include <cstdint>
#include <string_view>
#include <vector>

struct gbm_bo;
struct gbm_device;

namespace pl_compose {

struct Warp {
    float m[9] = {1, 0, 0, 0, 1, 0, 0, 0, 1};
};

struct SourceSlot {
    int src_y_fd = -1;
    int src_uv_fd = -1;
    int src_w = 0;
    int src_h = 0;
    int src_y_pitch = 0;
    int src_uv_pitch = 0;
    int src_y_offset = 0;  // byte offset of the Y plane within src_y_fd
    int src_uv_offset = 0; // byte offset of the UV plane within src_uv_fd
    int x = 0;
    int y = 0;
    int w = 0;
    int h = 0;
    Warp warp;
    int rotation = 0;       // 0, 90, 180, 270 clockwise degrees
    bool src_bt709 = false; // input YCbCr matrix (false = BT.601)
    float src_crop_x0 = 0.0F;
    float src_crop_y0 = 0.0F;
    float src_crop_x1 = 1.0F;
    float src_crop_y1 = 1.0F;
};

class PlCompose {
  public:
    PlCompose() = default;
    ~PlCompose();
    PlCompose(const PlCompose&) = delete;
    PlCompose& operator=(const PlCompose&) = delete;

    [[nodiscard]] bool init(std::string_view device_path, int canvas_w, int canvas_h);

    [[nodiscard]] bool render(const std::vector<SourceSlot>& slots);
    void finish();
    void swap();

    [[nodiscard]] int canvas_dmabuf_fd() const { return canvas_fd_[back_]; }
    [[nodiscard]] int canvas_w() const { return canvas_w_; }
    [[nodiscard]] int canvas_h() const { return canvas_h_; }
    [[nodiscard]] uint32_t canvas_stride() const { return canvas_stride_; }
    [[nodiscard]] gbm_bo* canvas_bo() const { return canvas_bo_[back_]; }
    [[nodiscard]] gbm_device* gbm() const;

  private:
    struct Impl;
    [[nodiscard]] bool init_backend();
    [[nodiscard]] bool alloc_canvas(int canvas_w, int canvas_h);

    gsl::owner<Impl*> impl_ = nullptr;
    int canvas_w_ = 0;
    int canvas_h_ = 0;
    uint32_t canvas_stride_ = 0;
    // Must be >= max concurrent consumers + 1 (broadcast fds are zero-copy).
    static constexpr int kBufCount = 4;
    int back_ = 0;
    gbm_bo* canvas_bo_[kBufCount] = {};
    int canvas_fd_[kBufCount] = {-1, -1, -1, -1};
    bool cpu_clear_ = false; // VIDEONODE_COMPOSER_CPU_CLEAR fallback (sync hazard)
};

} // namespace pl_compose
