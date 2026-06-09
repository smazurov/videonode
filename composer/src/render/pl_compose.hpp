// pl_compose — libplacebo multi-source GPU compositor.
//
// Replaces gl_compose. Owns a BGRA canvas dma-buf, imports per-source
// NV12 dma-bufs as pl_tex, composites via pl_dispatch with an optional
// per-source 3×3 homography warp. Backend-agnostic (OpenGL today,
// Vulkan later).

#pragma once

#include "src/common/owner.hpp"
#include "src/render/source_slot.hpp"

#include <cstdint>
#include <string_view>
#include <vector>

struct gbm_bo;
struct gbm_device;

namespace pl_compose {

class PlCompose {
  public:
    PlCompose() = default;
    ~PlCompose();
    PlCompose(const PlCompose&) = delete;
    PlCompose& operator=(const PlCompose&) = delete;

    [[nodiscard]] bool init(std::string_view device_path, int canvas_w, int canvas_h);

    // background_rgba is packed 0xRRGGBBAA; the canvas is cleared to it
    // before any slot is composited. Default opaque black.
    [[nodiscard]] bool render(const std::vector<SourceSlot>& slots,
                              uint32_t background_rgba = 0x000000FFU);
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
