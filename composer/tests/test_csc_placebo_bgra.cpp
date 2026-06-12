#include "src/render/csc.hpp"
#include "src/render/csc_placebo.hpp"
#include "src/render/gbm_alloc.hpp"

#include <gtest/gtest.h>

#include <gbm.h>

#include <cstdint>
#include <span>

namespace {

constexpr int kW = 64;
constexpr int kH = 64;

// Solid red in BT.709 limited: Y=63, Cb=102, Cr=240.
constexpr uint8_t kRedY = 63;
constexpr uint8_t kRedCb = 102;
constexpr uint8_t kRedCr = 240;

bool fill_bgra_red(gbm_alloc::Nv12Buf& src) {
    auto m = gbm_alloc::map_rw(src);
    if (!m.y)
        return false;
    auto y = m.y_bytes();
    for (int r = 0; r < kH; ++r) {
        auto row = y.subspan(static_cast<size_t>(r) * m.y_stride, static_cast<size_t>(kW) * 4);
        for (int p = 0; p < kW; ++p) {
            auto px = row.subspan(static_cast<size_t>(p) * 4, 4);
            px[0] = 0;
            px[1] = 0;
            px[2] = 255;
            px[3] = 255;
        }
    }
    gbm_alloc::unmap(src);
    return true;
}

} // namespace

// The composer's canvas is BGRA; a GLES libplacebo context has no sampleable
// bgra8 format, so the import falls back to rgba8 — without a compensating
// component mapping every composed output ships with R and B exchanged.
TEST(CscPlaceboBgra, RedCanvasConvertsToRedNv12) {
    if (!csc_placebo::init())
        GTEST_SKIP() << "no DRM render node";
    gbm_device* gbm = csc_placebo::gbm_device_for_io();
    ASSERT_NE(gbm, nullptr);

    gbm_alloc::Nv12Buf src = gbm_alloc::alloc(gbm, 4 * kW, kH);
    gbm_alloc::Nv12Buf dst = gbm_alloc::alloc(gbm, kW, kH);
    ASSERT_TRUE(src.valid());
    ASSERT_TRUE(dst.valid());
    ASSERT_TRUE(fill_bgra_red(src));

    csc::ConvertParams sp;
    sp.fd = src.y_fd;
    sp.fmt = csc::PixelFormat::Bgra;
    sp.width = kW;
    sp.height = kH;
    sp.wstride = static_cast<int>(src.y_stride);

    csc::ConvertParams dp;
    dp.fd = dst.y_fd;
    dp.uv_fd = dst.uv_fd;
    dp.fmt = csc::PixelFormat::Nv12;
    dp.width = kW;
    dp.height = kH;
    dp.wstride = static_cast<int>(dst.y_stride);
    dp.uv_wstride = static_cast<int>(dst.uv_stride);
    dp.color_space = csc::ColorSpace::Bt709Limited;

    ASSERT_TRUE(csc_placebo::convert(sp, dp));

    auto m = gbm_alloc::map_rw(dst);
    ASSERT_NE(m.y, nullptr);
    ASSERT_NE(m.uv, nullptr);
    const size_t y_mid = static_cast<size_t>(kH / 2) * m.y_stride + kW / 2;
    const size_t uv_mid = static_cast<size_t>(kH / 4) * m.uv_stride + (kW / 4) * 2;
    EXPECT_NEAR(m.y_bytes()[y_mid], kRedY, 6);
    EXPECT_NEAR(m.uv_bytes()[uv_mid], kRedCb, 8) << "Cb wrong: R/B swapped in BGRA import";
    EXPECT_NEAR(m.uv_bytes()[uv_mid + 1], kRedCr, 8) << "Cr wrong: R/B swapped in BGRA import";
    gbm_alloc::unmap(dst);

    gbm_alloc::free(src);
    gbm_alloc::free(dst);
}
