// Tests for csc_gles::convert(). Drive the production backend with
// GBM-allocated dma-bufs on a Mesa render node, verifying byte-exact
// output for both supported source formats (NV12 → NV12 copy, NV24 →
// NV12 downsample) and the rejection paths.
//
// Test is gated on egl_ctx::EglCtx::init() succeeding — on headless CI
// without /dev/dri/renderD12{8,9,30}, all GBM-dependent cases gtest-skip.

#include "src/render/csc.hpp"
#include "src/render/csc_gles.hpp"

#include <drm_fourcc.h>
#include <gbm.h>
#include <gtest/gtest.h>

#include <cstdint>
#include <cstdlib>

namespace {

// Test buffer allocation must share csc_gles's internal gbm_device.
// Cross-gbm_device dma-buf import as a renderbuffer fails on radeonsi
// (Mesa rejects the FBO completeness check), so the test allocates via
// csc_gles::gbm_device_for_io() after csc_gles::init() succeeds.

// R8 GBM buffer used as backing storage for NV12 or NV24 dma-bufs.
// Allocated as a single R8 plane of width=img_w, height=total_rows so
// the entire image (Y + UV stacked) lives in one fd.
struct R8Bo {
    gbm_bo* bo = nullptr;
    int fd = -1;
    uint32_t stride = 0;
    int w = 0;
    int total_h = 0;
    void* map_handle = nullptr;
    void* mapped = nullptr;

    ~R8Bo() {
        if (mapped)
            gbm_bo_unmap(bo, map_handle);
        if (fd >= 0)
            ::close(fd);
        if (bo)
            gbm_bo_destroy(bo);
    }

    bool alloc(gbm_device* d, int img_w, int total_rows) {
        bo = gbm_bo_create(d, img_w, total_rows, DRM_FORMAT_R8,
                           GBM_BO_USE_LINEAR | GBM_BO_USE_RENDERING);
        if (!bo)
            return false;
        fd = gbm_bo_get_fd(bo);
        stride = gbm_bo_get_stride(bo);
        w = img_w;
        total_h = total_rows;
        return fd >= 0;
    }

    uint8_t* map_rw() {
        uint32_t s = 0;
        mapped = gbm_bo_map(bo, 0, 0, w, total_h, GBM_BO_TRANSFER_READ_WRITE, &s, &map_handle);
        return static_cast<uint8_t*>(mapped);
    }

    uint8_t* map_read() {
        uint32_t s = 0;
        mapped = gbm_bo_map(bo, 0, 0, w, total_h, GBM_BO_TRANSFER_READ, &s, &map_handle);
        return static_cast<uint8_t*>(mapped);
    }

    void unmap() {
        if (mapped) {
            gbm_bo_unmap(bo, map_handle);
            mapped = nullptr;
            map_handle = nullptr;
        }
    }
};

// Test fixture: brings csc_gles up once. If the host has no usable DRM
// render node, every body-bearing case calls GTEST_SKIP.
class CscGlesTest : public ::testing::Test {
  protected:
    void SetUp() override {
        if (!csc_gles::init()) {
            GTEST_SKIP() << "csc_gles::init failed — no Mesa render node on this host";
        }
        gbm_dev_ = csc_gles::gbm_device_for_io();
        ASSERT_NE(gbm_dev_, nullptr);
    }

    gbm_device* gbm_dev_ = nullptr;
};

constexpr int kW = 64;
constexpr int kH = 32;

uint8_t y_pattern(int x, int y) {
    return static_cast<uint8_t>((x * 3 + y * 5) & 0xFF);
}
uint8_t u_pattern(int x, int y) {
    return static_cast<uint8_t>((x ^ (y * 2)) & 0xFF);
}
uint8_t v_pattern(int x, int y) {
    return static_cast<uint8_t>((x * 7 + y * 11) & 0xFF);
}

} // namespace

TEST_F(CscGlesTest, Nv12ToNv12ByteExactCopy) {
    // Split-buffer layout: Y is one R8 BO (kW × kH); UV is a second R8 BO
    // sized to carry kW/2 GR88 samples per row × kH/2 rows. That matches
    // the host gbm_alloc::alloc() production allocator (two independent
    // dma-bufs), and is the case csc_gles needs to handle via uv_fd.
    R8Bo src_y, src_uv, dst_y, dst_uv;
    ASSERT_TRUE(src_y.alloc(gbm_dev_, kW, kH));
    ASSERT_TRUE(src_uv.alloc(gbm_dev_, kW, kH / 2)); // 1 GR88 sample = 2 R8 bytes
    ASSERT_TRUE(dst_y.alloc(gbm_dev_, kW, kH));
    ASSERT_TRUE(dst_uv.alloc(gbm_dev_, kW, kH / 2));

    const int src_y_stride = static_cast<int>(src_y.stride);
    const int src_uv_stride = static_cast<int>(src_uv.stride);
    const int dst_y_stride = static_cast<int>(dst_y.stride);
    const int dst_uv_stride = static_cast<int>(dst_uv.stride);

    uint8_t* py = src_y.map_rw();
    ASSERT_NE(py, nullptr);
    for (int y = 0; y < kH; ++y) {
        for (int x = 0; x < kW; ++x)
            py[y * src_y_stride + x] = y_pattern(x, y);
    }
    src_y.unmap();

    uint8_t* puv = src_uv.map_rw();
    ASSERT_NE(puv, nullptr);
    for (int uvy = 0; uvy < kH / 2; ++uvy) {
        uint8_t* row = puv + uvy * src_uv_stride;
        for (int uvx = 0; uvx < kW / 2; ++uvx) {
            row[2 * uvx + 0] = u_pattern(uvx, uvy);
            row[2 * uvx + 1] = v_pattern(uvx, uvy);
        }
    }
    src_uv.unmap();

    csc::ConvertParams sp{}, dp{};
    sp.fd = src_y.fd;
    sp.uv_fd = src_uv.fd;
    sp.fmt = csc::PixelFormat::Nv12;
    sp.width = kW;
    sp.height = kH;
    sp.wstride = src_y_stride;
    sp.uv_wstride = src_uv_stride;
    dp.fd = dst_y.fd;
    dp.uv_fd = dst_uv.fd;
    dp.fmt = csc::PixelFormat::Nv12;
    dp.width = kW;
    dp.height = kH;
    dp.wstride = dst_y_stride;
    dp.uv_wstride = dst_uv_stride;
    ASSERT_TRUE(csc::convert(sp, dp));

    uint8_t* oy = dst_y.map_read();
    ASSERT_NE(oy, nullptr);
    int y_errors = 0;
    int first_y_err_x = -1, first_y_err_y = -1, first_y_got = -1, first_y_want = -1;
    for (int y = 0; y < kH; ++y) {
        for (int x = 0; x < kW; ++x) {
            uint8_t got = oy[y * dst_y_stride + x];
            uint8_t want = y_pattern(x, y);
            if (got != want) {
                if (y_errors == 0) {
                    first_y_err_x = x;
                    first_y_err_y = y;
                    first_y_got = got;
                    first_y_want = want;
                }
                ++y_errors;
            }
        }
    }
    dst_y.unmap();

    uint8_t* ouv = dst_uv.map_read();
    ASSERT_NE(ouv, nullptr);
    int uv_errors = 0;
    for (int uvy = 0; uvy < kH / 2; ++uvy) {
        uint8_t* row = ouv + uvy * dst_uv_stride;
        for (int uvx = 0; uvx < kW / 2; ++uvx) {
            uint8_t gu = row[2 * uvx + 0];
            uint8_t gv = row[2 * uvx + 1];
            if (gu != u_pattern(uvx, uvy) || gv != v_pattern(uvx, uvy))
                ++uv_errors;
        }
    }
    dst_uv.unmap();

    EXPECT_EQ(0, y_errors) << "first Y mismatch at (" << first_y_err_x << "," << first_y_err_y
                           << "): got " << first_y_got << " want " << first_y_want;
    EXPECT_EQ(0, uv_errors) << "UV pass-through mismatch — single-tap NV12 copy is not byte-exact";
}

TEST_F(CscGlesTest, Nv24ToNv12LumaPassthroughAndChromaDownsample) {
    // NV24 source: Y (R8, W×H) + UV (GR88 at full res, W×H samples = R8
    // 2W × H). NV12 dst: Y (R8, W×H) + UV (GR88 at half res = R8 W ×
    // H/2). All four allocated as independent BOs (split layout).
    R8Bo src_y, src_uv, dst_y, dst_uv;
    ASSERT_TRUE(src_y.alloc(gbm_dev_, kW, kH));
    ASSERT_TRUE(src_uv.alloc(gbm_dev_, kW * 2, kH));
    ASSERT_TRUE(dst_y.alloc(gbm_dev_, kW, kH));
    ASSERT_TRUE(dst_uv.alloc(gbm_dev_, kW, kH / 2));

    const int src_y_stride = static_cast<int>(src_y.stride);
    const int src_uv_stride = static_cast<int>(src_uv.stride);
    const int dst_y_stride = static_cast<int>(dst_y.stride);
    const int dst_uv_stride = static_cast<int>(dst_uv.stride);

    uint8_t* py = src_y.map_rw();
    ASSERT_NE(py, nullptr);
    for (int y = 0; y < kH; ++y) {
        for (int x = 0; x < kW; ++x)
            py[y * src_y_stride + x] = y_pattern(x, y);
    }
    src_y.unmap();

    uint8_t* puv = src_uv.map_rw();
    ASSERT_NE(puv, nullptr);
    for (int yy = 0; yy < kH; ++yy) {
        uint8_t* row = puv + yy * src_uv_stride;
        for (int xx = 0; xx < kW; ++xx) {
            row[2 * xx + 0] = u_pattern(xx, yy);
            row[2 * xx + 1] = v_pattern(xx, yy);
        }
    }
    src_uv.unmap();

    csc::ConvertParams sp{}, dp{};
    sp.fd = src_y.fd;
    sp.uv_fd = src_uv.fd;
    sp.fmt = csc::PixelFormat::Nv24;
    sp.width = kW;
    sp.height = kH;
    sp.wstride = src_y_stride;
    sp.uv_wstride = src_uv_stride;
    dp.fd = dst_y.fd;
    dp.uv_fd = dst_uv.fd;
    dp.fmt = csc::PixelFormat::Nv12;
    dp.width = kW;
    dp.height = kH;
    dp.wstride = dst_y_stride;
    dp.uv_wstride = dst_uv_stride;
    ASSERT_TRUE(csc::convert(sp, dp));

    // Y plane: byte-exact pass-through.
    uint8_t* oy = dst_y.map_read();
    ASSERT_NE(oy, nullptr);
    int y_errors = 0;
    for (int y = 0; y < kH; ++y) {
        for (int x = 0; x < kW; ++x) {
            if (oy[y * dst_y_stride + x] != y_pattern(x, y))
                ++y_errors;
        }
    }
    dst_y.unmap();

    // UV plane: 2×2 average of source UV. Each dst UV sample at (uvx, uvy)
    // averages source UV at the 2×2 block { (2uvx, 2uvy), (2uvx+1, 2uvy),
    // (2uvx, 2uvy+1), (2uvx+1, 2uvy+1) }. Allow ±1 LSB tolerance for GPU
    // rounding (the production probe applies the same tolerance).
    uint8_t* ouv = dst_uv.map_read();
    ASSERT_NE(ouv, nullptr);
    int uv_errors = 0;
    for (int uvy = 0; uvy < kH / 2; ++uvy) {
        uint8_t* row = ouv + uvy * dst_uv_stride;
        for (int uvx = 0; uvx < kW / 2; ++uvx) {
            int u_avg = (u_pattern(2 * uvx, 2 * uvy) + u_pattern(2 * uvx + 1, 2 * uvy) +
                         u_pattern(2 * uvx, 2 * uvy + 1) + u_pattern(2 * uvx + 1, 2 * uvy + 1)) /
                        4;
            int v_avg = (v_pattern(2 * uvx, 2 * uvy) + v_pattern(2 * uvx + 1, 2 * uvy) +
                         v_pattern(2 * uvx, 2 * uvy + 1) + v_pattern(2 * uvx + 1, 2 * uvy + 1)) /
                        4;
            int du = std::abs(static_cast<int>(row[2 * uvx + 0]) - u_avg);
            int dv = std::abs(static_cast<int>(row[2 * uvx + 1]) - v_avg);
            if (du > 1 || dv > 1)
                ++uv_errors;
        }
    }
    dst_uv.unmap();

    EXPECT_EQ(0, y_errors) << "Y plane diverged from NV24 source";
    EXPECT_EQ(0, uv_errors) << "UV downsample out of ±1 LSB tolerance";
}

TEST_F(CscGlesTest, RejectsUnsupportedSrcFormat) {
    R8Bo dst;
    ASSERT_TRUE(dst.alloc(gbm_dev_, kW, kH + kH / 2));
    csc::ConvertParams sp{}, dp{};
    sp.fd = dst.fd; // unused beyond the early reject
    sp.fmt = csc::PixelFormat::Yuyv;
    sp.width = kW;
    sp.height = kH;
    dp.fd = dst.fd;
    dp.fmt = csc::PixelFormat::Nv12;
    dp.width = kW;
    dp.height = kH;
    EXPECT_FALSE(csc_gles::convert(sp, dp));
}

TEST_F(CscGlesTest, RejectsNonNv12Dst) {
    R8Bo dst;
    ASSERT_TRUE(dst.alloc(gbm_dev_, kW, kH + kH / 2));
    csc::ConvertParams sp{}, dp{};
    sp.fd = dst.fd;
    sp.fmt = csc::PixelFormat::Nv12;
    sp.width = kW;
    sp.height = kH;
    dp.fd = dst.fd;
    dp.fmt = csc::PixelFormat::Nv24;
    dp.width = kW;
    dp.height = kH;
    EXPECT_FALSE(csc_gles::convert(sp, dp));
}

TEST_F(CscGlesTest, RejectsOddDimensions) {
    R8Bo dst;
    ASSERT_TRUE(dst.alloc(gbm_dev_, kW, kH + kH / 2));
    csc::ConvertParams sp{}, dp{};
    sp.fd = dst.fd;
    sp.fmt = csc::PixelFormat::Nv12;
    sp.width = kW + 1; // odd
    sp.height = kH;
    dp.fd = dst.fd;
    dp.fmt = csc::PixelFormat::Nv12;
    dp.width = kW + 1;
    dp.height = kH;
    EXPECT_FALSE(csc_gles::convert(sp, dp));
}
