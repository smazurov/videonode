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
    int x = 0;
    int y = 0;
    int w = 0;
    int h = 0;
    Warp warp;
    int rotation = 0; // 0, 90, 180, 270 clockwise degrees
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

    [[nodiscard]] int canvas_dmabuf_fd() const { return canvas_fd_; }
    [[nodiscard]] int canvas_w() const { return canvas_w_; }
    [[nodiscard]] int canvas_h() const { return canvas_h_; }
    [[nodiscard]] uint32_t canvas_stride() const { return canvas_stride_; }
    [[nodiscard]] gbm_bo* canvas_bo() const { return canvas_bo_; }
    [[nodiscard]] gbm_device* gbm() const;

  private:
    struct Impl;
    gsl::owner<Impl*> impl_ = nullptr;
    int canvas_w_ = 0;
    int canvas_h_ = 0;
    uint32_t canvas_stride_ = 0;
    gbm_bo* canvas_bo_ = nullptr;
    int canvas_fd_ = -1;
};

} // namespace pl_compose
