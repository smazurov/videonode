// Asserts both canvas-clear paths zero the canvas. The GPU clear
// (pl_tex_clear) is the steady-state path; the CPU memset is the
// VIDEONODE_COMPOSER_CPU_CLEAR fallback. Alpha is ignored — only RGB
// reaches the encoder.

#include "src/render/gbm_alloc.hpp"
#include "src/render/pl_compose.hpp"

#include <gtest/gtest.h>

#include <gbm.h>

#include <cstdint>
#include <cstdlib>
#include <mutex>
#include <span>

namespace {

constexpr int kW = 64;
constexpr int kH = 64;

void expect_black_canvas(const char* why) {
    pl_compose::PlCompose comp;
    if (!comp.init("/dev/dri/renderD128", kW, kH)) {
        GTEST_SKIP() << "PlCompose::init failed — no DRM render node";
    }
    ASSERT_TRUE(comp.render({})) << why;
    comp.finish();

    uint32_t map_stride = 0;
    void* handle = nullptr;
    void* px = nullptr;
    {
        std::lock_guard<std::mutex> g(gbm_alloc::gbm_device_mu());
        px = gbm_bo_map(comp.canvas_bo(), 0, 0, kW, kH, GBM_BO_TRANSFER_READ, &map_stride, &handle);
    }
    ASSERT_NE(px, nullptr) << why;

    int nonblack = 0;
    const auto* base_ptr = static_cast<const uint8_t*>(px);
    const size_t total_bytes = size_t(kH) * map_stride;
    const auto buf = std::span<const uint8_t>(base_ptr, total_bytes);
    for (int y = 0; y < kH; ++y) {
        const auto row = buf.subspan(size_t(y) * map_stride, map_stride);
        for (int x = 0; x < kW; ++x) {
            const auto pixel = row.subspan(size_t(x) * 4, 4);
            if (pixel[0] != 0 || pixel[1] != 0 || pixel[2] != 0)
                ++nonblack;
        }
    }
    {
        std::lock_guard<std::mutex> g(gbm_alloc::gbm_device_mu());
        gbm_bo_unmap(comp.canvas_bo(), handle);
    }
    EXPECT_EQ(nonblack, 0) << why << ": " << nonblack << " non-black pixels";
}

} // namespace

TEST(CanvasClear, GpuClearProducesBlackCanvas) {
    ::unsetenv("VIDEONODE_COMPOSER_CPU_CLEAR");
    expect_black_canvas("gpu clear");
}

TEST(CanvasClear, CpuClearFallbackProducesBlackCanvas) {
    ::setenv("VIDEONODE_COMPOSER_CPU_CLEAR", "1", 1);
    expect_black_canvas("cpu clear fallback");
    ::unsetenv("VIDEONODE_COMPOSER_CPU_CLEAR");
}
