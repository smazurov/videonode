// probe-dmaheap-render — answers one question on this rig's Panthor GPU:
// can libplacebo RENDER NV12 output into a dma_heap-backed dma-buf used as
// an FBO color attachment (Option A), or does it require a GBM render
// target (Option B)?
//
// TEST A: NV24 dma_heap source -> NV12 dma_heap dst (single-bo). convert()
//   returning true with sane pixels => Option A works.
// TEST B (control): same source -> NV12 GBM dst (split planes). Proves the
//   GPU + CSC path itself is healthy independent of the dst allocator.
//
// Developer-only probe; only built when HAVE_PLACEBO AND HAVE_GBM.

#include "src/ipc/dma_heap.hpp"
#include "src/render/csc.hpp"
#include "src/render/csc_placebo.hpp"
#include "src/render/gbm_alloc.hpp"
#include "src/render/nv12_buf.hpp"

#include <cstddef>
#include <cstdint>
#include <cstdio>
#include <gbm.h>
#include <span>
#include <sys/mman.h>
#include <unistd.h>

namespace {

constexpr int kW = 256;
constexpr int kH = 256;

struct YuvStats {
    double mean_y = 0.0;
    int uv_samples[4][2] = {};
};

void fill_nv24_src(std::span<uint8_t> base, uint32_t stride) {
    const size_t y_size = static_cast<size_t>(stride) * kH;
    std::span<uint8_t> y = base.first(y_size);
    std::span<uint8_t> uv = base.subspan(y_size);
    for (int row = 0; row < kH; ++row)
        for (int col = 0; col < kW; ++col)
            y[static_cast<size_t>(row) * stride + col] =
                static_cast<uint8_t>(16 + (219 * col) / (kW - 1));
    // NV24: full-res interleaved U/V, two bytes per pixel. Saturated chroma
    // (U=84, V=255) so a real conversion is obvious vs zeros/garbage.
    for (int row = 0; row < kH; ++row) {
        for (int col = 0; col < kW; ++col) {
            const size_t o = static_cast<size_t>(row) * stride * 2 + static_cast<size_t>(col) * 2;
            uv[o] = 84;
            uv[o + 1] = 255;
        }
    }
}

bool make_nv24_source(dmaheap::Buffer& bo, uint32_t& stride_out) {
    const uint32_t stride = nv12_buf::aligned_stride(kW);
    const size_t size = static_cast<size_t>(stride) * kH * 3; // Y + 2*UV
    bo = dmaheap::alloc(dmaheap::kHeapSystem, size);
    if (!bo.valid())
        bo = dmaheap::alloc(dmaheap::kHeapUncached, size);
    if (!bo.valid()) {
        std::fprintf(stderr, "make_nv24_source: dma_heap alloc failed\n");
        return false;
    }
    void* p = dmaheap::mmap_rw(bo);
    if (!p) {
        std::fprintf(stderr, "make_nv24_source: mmap failed\n");
        return false;
    }
    dmaheap::sync_start(bo.fd.get(), dmaheap::SyncDir::Write);
    fill_nv24_src(std::span<uint8_t>{static_cast<uint8_t*>(p), bo.size}, stride);
    dmaheap::sync_end(bo.fd.get(), dmaheap::SyncDir::Write);
    dmaheap::munmap_rw(p, bo.size);
    stride_out = stride;
    return true;
}

csc::ConvertParams src_params(int fd, uint32_t stride) {
    csc::ConvertParams p;
    p.fd = fd;
    p.uv_fd = -1;
    p.fmt = csc::PixelFormat::Nv24;
    p.width = kW;
    p.height = kH;
    p.wstride = static_cast<int>(stride);
    return p;
}

YuvStats read_nv12(std::span<const uint8_t> y, uint32_t y_pitch, std::span<const uint8_t> uv,
                   uint32_t uv_pitch) {
    YuvStats s;
    double acc = 0.0;
    for (int row = 0; row < kH; ++row)
        for (int col = 0; col < kW; ++col)
            acc += y[static_cast<size_t>(row) * y_pitch + col];
    s.mean_y = acc / (static_cast<double>(kW) * kH);
    const int sx[4] = {0, kW / 4, kW / 2, kW - 2};
    for (int i = 0; i < 4; ++i) {
        const size_t o =
            static_cast<size_t>(kH / 4) * uv_pitch + static_cast<size_t>(sx[i] / 2) * 2;
        s.uv_samples[i][0] = uv[o];
        s.uv_samples[i][1] = uv[o + 1];
    }
    return s;
}

void print_stats(const char* tag, const YuvStats& s) {
    std::printf("  %s: mean_Y=%.2f  UV[", tag, s.mean_y);
    for (int i = 0; i < 4; ++i)
        std::printf("(%d,%d)%s", s.uv_samples[i][0], s.uv_samples[i][1], i < 3 ? " " : "");
    std::printf("]\n");
}

bool test_a_dmaheap(const csc::ConvertParams& src) {
    std::printf("--- TEST A: dma_heap render target ---\n");
    const uint32_t stride = nv12_buf::aligned_stride(kW);
    const size_t size = static_cast<size_t>(stride) * kH * 3 / 2;
    dmaheap::Buffer dst = dmaheap::alloc(dmaheap::kHeapSystem, size);
    if (!dst.valid())
        dst = dmaheap::alloc(dmaheap::kHeapUncached, size);
    if (!dst.valid()) {
        std::fprintf(stderr, "TEST A: dma_heap dst alloc failed\n");
        return false;
    }

    csc::ConvertParams dp;
    dp.fd = dst.fd.get();
    dp.uv_fd = -1;
    dp.fmt = csc::PixelFormat::Nv12;
    dp.width = kW;
    dp.height = kH;
    dp.wstride = static_cast<int>(stride);

    bool ok = csc_placebo::convert(src, dp);
    std::printf("  convert() returned: %s\n", ok ? "TRUE" : "FALSE");
    if (!ok)
        return false;

    void* p = dmaheap::mmap_rw(dst);
    if (!p) {
        std::fprintf(stderr, "TEST A: mmap dst failed\n");
        return false;
    }
    dmaheap::sync_start(dst.fd.get(), dmaheap::SyncDir::Read);
    std::span<const uint8_t> base{static_cast<const uint8_t*>(p), dst.size};
    YuvStats s = read_nv12(base, stride, base.subspan(static_cast<size_t>(stride) * kH), stride);
    dmaheap::sync_end(dst.fd.get(), dmaheap::SyncDir::Read);
    print_stats("readback", s);
    dmaheap::munmap_rw(p, dst.size);
    return s.mean_y > 1.0;
}

std::span<const uint8_t> map_ro(int fd, size_t size) {
    void* p = ::mmap(nullptr, size, PROT_READ, MAP_SHARED, fd, 0);
    if (p == MAP_FAILED)
        return {};
    return {static_cast<const uint8_t*>(p), size};
}

bool test_b_gbm(const csc::ConvertParams& src) {
    std::printf("--- TEST B: GBM render target (control) ---\n");
    gbm_device* gbm = csc_placebo::gbm_device_for_io();
    if (!gbm) {
        std::fprintf(stderr, "TEST B: gbm_device_for_io() returned null\n");
        return false;
    }
    gbm_alloc::Nv12Buf dst = gbm_alloc::alloc(gbm, kW, kH);
    if (!dst.valid()) {
        std::fprintf(stderr, "TEST B: gbm_alloc::alloc failed\n");
        return false;
    }

    csc::ConvertParams dp;
    dp.fd = dst.y_fd;
    dp.uv_fd = dst.uv_fd;
    dp.fmt = csc::PixelFormat::Nv12;
    dp.width = kW;
    dp.height = kH;
    dp.wstride = static_cast<int>(dst.y_stride);
    dp.uv_wstride = static_cast<int>(dst.uv_stride);

    bool ok = csc_placebo::convert(src, dp);
    std::printf("  convert() returned: %s\n", ok ? "TRUE" : "FALSE");
    if (!ok) {
        gbm_alloc::free(dst);
        return false;
    }

    const size_t y_size = static_cast<size_t>(dst.y_stride) * kH;
    const size_t uv_size = static_cast<size_t>(dst.uv_stride) * (kH / 2);
    std::span<const uint8_t> yp = map_ro(dst.y_fd, y_size);
    std::span<const uint8_t> up = map_ro(dst.uv_fd, uv_size);
    bool pass = false;
    if (!yp.empty() && !up.empty()) {
        YuvStats s = read_nv12(yp, dst.y_stride, up, dst.uv_stride);
        print_stats("readback", s);
        pass = s.mean_y > 1.0;
    } else {
        std::fprintf(stderr, "TEST B: mmap gbm planes failed\n");
    }
    if (!yp.empty())
        ::munmap(const_cast<uint8_t*>(yp.data()), y_size);
    if (!up.empty())
        ::munmap(const_cast<uint8_t*>(up.data()), uv_size);
    gbm_alloc::free(dst);
    return pass;
}

} // namespace

int main() {
    std::printf("probe-dmaheap-render: NV24 -> NV12 at %dx%d on Panthor\n", kW, kH);

    if (!csc_placebo::init()) {
        std::fprintf(stderr, "csc_placebo::init() failed — no usable render node\n");
        std::printf("VERDICT: dma_heap_render=FAIL gbm_render=FAIL\n");
        return 1;
    }

    dmaheap::Buffer src_bo;
    uint32_t src_stride = 0;
    if (!make_nv24_source(src_bo, src_stride)) {
        std::printf("VERDICT: dma_heap_render=FAIL gbm_render=FAIL\n");
        return 1;
    }
    csc::ConvertParams src = src_params(src_bo.fd.get(), src_stride);

    bool a = test_a_dmaheap(src);
    bool b = test_b_gbm(src);

    std::printf("VERDICT: dma_heap_render=%s gbm_render=%s\n", a ? "PASS" : "FAIL",
                b ? "PASS" : "FAIL");
    return (a || b) ? 0 : 1;
}
