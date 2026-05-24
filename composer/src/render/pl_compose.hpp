// pl_compose — libplacebo multi-source GPU compositor.
//
// Replaces gl_compose. Owns a BGRA canvas dma-buf, imports per-source
// NV12 dma-bufs as pl_tex, composites via pl_dispatch with an optional
// per-source 3×3 homography warp. Backend-agnostic (OpenGL today,
// Vulkan later).

#pragma once

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
};

class PlCompose {
  public:
    [[nodiscard]] bool init(std::string_view device_path, int canvas_w, int canvas_h);
    ~PlCompose();

    PlCompose() = default;
    PlCompose(const PlCompose&) = delete;
    PlCompose& operator=(const PlCompose&) = delete;

    [[nodiscard]] bool render(const std::vector<SourceSlot>& slots);
    void finish();

    int canvas_dmabuf_fd() const { return canvas_fd_; }
    int canvas_w() const { return canvas_w_; }
    int canvas_h() const { return canvas_h_; }
    uint32_t canvas_stride() const { return canvas_stride_; }
    gbm_bo* canvas_bo() const { return canvas_bo_; }
    gbm_device* gbm() const;

  private:
    struct Impl;
    Impl* impl_ = nullptr;
    int canvas_w_ = 0;
    int canvas_h_ = 0;
    uint32_t canvas_stride_ = 0;
    gbm_bo* canvas_bo_ = nullptr;
    int canvas_fd_ = -1;
};

} // namespace pl_compose
