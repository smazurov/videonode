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

struct Color {
    uint8_t y;
    uint8_t u;
    uint8_t v;
};

// Packed 4:2:2 lives in the Y plane of a 2*W-wide R8 bo, giving W*2 bytes/row
// with no separate-plane offset to reason about.
bool fill_packed(gbm_alloc::Nv12Buf& src, Color c, bool uyvy) {
    auto m = gbm_alloc::map_rw(src);
    if (!m.y)
        return false;
    auto y = m.y_bytes();
    for (int r = 0; r < kH; ++r) {
        auto row = y.subspan(static_cast<size_t>(r) * m.y_stride, static_cast<size_t>(kW) * 2);
        for (int p = 0; p < kW / 2; ++p) {
            auto quad = row.subspan(static_cast<size_t>(p) * 4, 4);
            quad[0] = uyvy ? c.u : c.y;
            quad[1] = uyvy ? c.y : c.u;
            quad[2] = uyvy ? c.v : c.y;
            quad[3] = uyvy ? c.y : c.v;
        }
    }
    gbm_alloc::unmap(src);
    return true;
}

csc::ConvertParams src_params(const gbm_alloc::Nv12Buf& src, csc::PixelFormat fmt) {
    csc::ConvertParams p;
    p.fd = src.y_fd;
    p.fmt = fmt;
    p.width = kW;
    p.height = kH;
    p.wstride = static_cast<int>(src.y_stride);
    return p;
}

csc::ConvertParams dst_params(const gbm_alloc::Nv12Buf& dst) {
    csc::ConvertParams p;
    p.fd = dst.y_fd;
    p.uv_fd = dst.uv_fd;
    p.fmt = csc::PixelFormat::Nv12;
    p.width = kW;
    p.height = kH;
    p.wstride = static_cast<int>(dst.y_stride);
    p.uv_wstride = static_cast<int>(dst.uv_stride);
    return p;
}

void check_center(gbm_alloc::Nv12Buf& dst, Color c) {
    auto m = gbm_alloc::map_rw(dst);
    ASSERT_NE(m.y, nullptr);
    ASSERT_NE(m.uv, nullptr);
    const size_t y_mid = static_cast<size_t>(kH / 2) * m.y_stride + kW / 2;
    const size_t uv_mid = static_cast<size_t>(kH / 4) * m.uv_stride + (kW / 4) * 2;
    EXPECT_NEAR(m.y_bytes()[y_mid], c.y, 6);
    EXPECT_NEAR(m.uv_bytes()[uv_mid], c.u, 8);
    EXPECT_NEAR(m.uv_bytes()[uv_mid + 1], c.v, 8);
    gbm_alloc::unmap(dst);
}

void expect_roundtrip(csc::PixelFormat fmt, bool uyvy) {
    if (!csc_placebo::init())
        GTEST_SKIP() << "no DRM render node";
    gbm_device* gbm = csc_placebo::gbm_device_for_io();
    ASSERT_NE(gbm, nullptr);

    gbm_alloc::Nv12Buf src = gbm_alloc::alloc(gbm, 2 * kW, kH);
    gbm_alloc::Nv12Buf dst = gbm_alloc::alloc(gbm, kW, kH);
    ASSERT_TRUE(src.valid());
    ASSERT_TRUE(dst.valid());

    const Color c{.y = 100, .u = 160, .v = 80};
    ASSERT_TRUE(fill_packed(src, c, uyvy));
    ASSERT_TRUE(csc_placebo::convert(src_params(src, fmt), dst_params(dst)));
    check_center(dst, c);

    gbm_alloc::free(src);
    gbm_alloc::free(dst);
}

} // namespace

TEST(CscPlaceboYuyv, YuyvRoundtripPreservesColor) {
    expect_roundtrip(csc::PixelFormat::Yuyv, /*uyvy=*/false);
}

TEST(CscPlaceboYuyv, UyvyRoundtripPreservesColor) {
    expect_roundtrip(csc::PixelFormat::Uyvy, /*uyvy=*/true);
}
