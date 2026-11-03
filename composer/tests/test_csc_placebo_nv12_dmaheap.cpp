// Raw UVC NV12 capture lands in a system dma-heap buffer with the driver's
// tight pitch (bytesperline == width) and UV packed right after Y. On Mesa
// hosts csc_placebo converts it into the GBM output ring. radeonsi refuses to
// import a linear plane whose pitch isn't 256-byte aligned, so a 1920-wide
// frame must still convert (via the CPU-upload fallback) like a 1280-wide one.

#include "src/ipc/dma_heap.hpp"
#include "src/render/csc.hpp"
#include "src/render/csc_placebo.hpp"
#include "src/render/gbm_alloc.hpp"

#include <gtest/gtest.h>

#include <gbm.h>

#include <cstdint>
#include <cstring>
#include <span>

namespace {

struct Color {
    uint8_t u;
    uint8_t v;
};

// Luma ramps with the row so a wrong row pitch shows up as a wrong value,
// not just a shifted copy of a flat fill.
uint8_t luma_for_row(int row, int h, int base) {
    return static_cast<uint8_t>(base + (row * 100 / h));
}

bool fill_tight_nv12(const dmaheap::Buffer& buf, int w, int h, int luma_base, Color c) {
    void* p = dmaheap::mmap_rw(buf);
    if (p == nullptr)
        return false;
    auto bytes = std::span(static_cast<uint8_t*>(p), buf.size);
    for (int r = 0; r < h; ++r) {
        auto row = bytes.subspan(static_cast<size_t>(r) * w, static_cast<size_t>(w));
        std::memset(row.data(), luma_for_row(r, h, luma_base), row.size());
    }
    const size_t y_size = static_cast<size_t>(w) * h;
    auto uv = bytes.subspan(y_size, y_size / 2);
    for (size_t i = 0; i + 1 < uv.size(); i += 2) {
        uv[i] = c.u;
        uv[i + 1] = c.v;
    }
    dmaheap::sync_end(buf.fd.get(), dmaheap::SyncDir::Write);
    dmaheap::munmap_rw(p, buf.size);
    return true;
}

void check_dst(gbm_alloc::Nv12Buf& dst, int w, int h, int luma_base, Color c) {
    auto m = gbm_alloc::map_rw(dst);
    ASSERT_NE(m.y, nullptr);
    ASSERT_NE(m.uv, nullptr);
    for (int row : {h / 4, h / 2, (3 * h) / 4}) {
        const size_t y_at = static_cast<size_t>(row) * m.y_stride + w / 2;
        EXPECT_NEAR(m.y_bytes()[y_at], luma_for_row(row, h, luma_base), 3) << "row " << row;
    }
    const size_t uv_mid = static_cast<size_t>(h / 4) * m.uv_stride + (w / 4) * 2;
    EXPECT_NEAR(m.uv_bytes()[uv_mid], c.u, 8);
    EXPECT_NEAR(m.uv_bytes()[uv_mid + 1], c.v, 8);
    gbm_alloc::unmap(dst);
}

// Mirrors the source's raw-capture call: fd + fmt + dims only, so the pitch
// defaults to the width and UV sits at offset w*h in the same fd.
csc::ConvertParams tight_src_params(const dmaheap::Buffer& src, int w, int h) {
    csc::ConvertParams p;
    p.fd = src.fd.get();
    p.fmt = csc::PixelFormat::Nv12;
    p.width = w;
    p.height = h;
    return p;
}

csc::ConvertParams dst_params(const gbm_alloc::Nv12Buf& dst, int w, int h) {
    csc::ConvertParams p;
    p.fd = dst.y_fd;
    p.uv_fd = dst.uv_fd;
    p.fmt = csc::PixelFormat::Nv12;
    p.width = w;
    p.height = h;
    p.wstride = static_cast<int>(dst.y_stride);
    p.uv_wstride = static_cast<int>(dst.uv_stride);
    return p;
}

void convert_frame(const dmaheap::Buffer& src, gbm_alloc::Nv12Buf& dst, int w, int h, int luma_base,
                   Color c) {
    ASSERT_TRUE(fill_tight_nv12(src, w, h, luma_base, c));
    ASSERT_TRUE(csc_placebo::convert(tight_src_params(src, w, h), dst_params(dst, w, h)));
    check_dst(dst, w, h, luma_base, c);
}

void expect_roundtrip(int w, int h) {
    if (!csc_placebo::init())
        GTEST_SKIP() << "no DRM render node";
    gbm_device* gbm = csc_placebo::gbm_device_for_io();
    ASSERT_NE(gbm, nullptr);

    dmaheap::Buffer src = dmaheap::alloc(dmaheap::kHeapSystem, static_cast<size_t>(w) * h * 3 / 2);
    if (!src.valid())
        GTEST_SKIP() << "no /dev/dma_heap/system";
    gbm_alloc::Nv12Buf dst = gbm_alloc::alloc(gbm, w, h);
    ASSERT_TRUE(dst.valid());

    // Two frames with different content: the second exercises the steady
    // state (latched import decision, reused upload texture).
    convert_frame(src, dst, w, h, 40, {.u = 160, .v = 80});
    convert_frame(src, dst, w, h, 120, {.u = 90, .v = 200});

    gbm_alloc::free(dst);
}

} // namespace

TEST(CscPlaceboNv12DmaHeap, Width1280TightPitch) {
    expect_roundtrip(1280, 720);
}

TEST(CscPlaceboNv12DmaHeap, Width1920TightPitch) {
    expect_roundtrip(1920, 1080);
}
