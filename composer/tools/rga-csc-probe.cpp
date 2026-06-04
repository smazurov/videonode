// rga-csc-probe — single-conversion validation of rga::convert().
//
// Allocates one BGR3 source dma-buf filled with a known solid-color
// pattern, one NV12 destination dma-buf, runs rga::convert(), mmaps
// both buffers and checks the destination matches the expected NV12
// values for that input color. Exits 0 on success, 1 on failure.
//
// Use case: a failing rga::convert is invisible from the live HDMI
// pipeline (you just get black/no frames downstream). This probe makes
// the failure surface as a clear pass/fail with format/stride/rect
// numbers logged.
//
// Only built when HAVE_RGA is set (on the rig).

#include "src/ipc/dma_heap.hpp"
#include "src/render/rga_csc.hpp"

#include <cstdint>
#include <cstdio>
#include <cstring>
#include <sys/mman.h>
#include <unistd.h>

namespace {

void fill_bgr3_solid(uint8_t* buf, int w, int h, uint8_t b, uint8_t g, uint8_t r) {
    for (int i = 0; i < w * h; ++i) {
        buf[i * 3 + 0] = b;
        buf[i * 3 + 1] = g;
        buf[i * 3 + 2] = r;
    }
}

// expected_nv12_luma — limited-range ("TV" / "studio range") BT.601 luma
// for a known BGR pixel. Verified empirically against RGA's actual output:
// RGA produces Y in [16,235]. Earlier full-range expectation failed by
// exactly the limited-range offset (e.g. +12 for pure blue: 29 -> 41).
uint8_t expected_nv12_luma_bt601(uint8_t b, uint8_t g, uint8_t r) {
    int fr = (299 * r + 587 * g + 114 * b) / 1000;
    int lr = 16 + (219 * fr) / 255;
    if (lr < 0)
        lr = 0;
    if (lr > 255)
        lr = 255;
    return static_cast<uint8_t>(lr);
}

bool check_solid_color(int W, int H, uint8_t b, uint8_t g, uint8_t r) {
    const size_t bgr3_size = size_t(W) * H * 3;
    const size_t nv12_size = size_t(W) * H * 3 / 2;

    // "system-uncached" sidesteps the cache-coherency issue between CPU
    // mmap writes and RGA's DMA reads on ARM. With "system" the kernel
    // gives us cached memory; the writes from fill_bgr3_solid sit in CPU
    // cache, RGA reads stale zeros from physical memory. Uncached is
    // slightly slower for CPU access but correct without explicit sync.
    dmaheap::Buffer src = dmaheap::alloc("system-uncached", bgr3_size);
    dmaheap::Buffer dst = dmaheap::alloc("system-uncached", nv12_size);
    if (!src.valid() || !dst.valid()) {
        fprintf(stderr, "rga-csc-probe: dma_heap alloc failed\n");
        return false;
    }

    void* sp = ::mmap(nullptr, bgr3_size, PROT_READ | PROT_WRITE, MAP_SHARED, src.fd, 0);
    if (sp == MAP_FAILED) {
        fprintf(stderr, "mmap src: failed\n");
        return false;
    }
    fill_bgr3_solid(static_cast<uint8_t*>(sp), W, H, b, g, r);
    ::munmap(sp, bgr3_size);

    rga::ConvertParams sp_p, dp_p;
    sp_p.fd = src.fd;
    sp_p.fmt = rga::PixelFormat::Bgr3;
    sp_p.width = W;
    sp_p.height = H;
    // librga's wstride is PIXEL stride (not byte stride). For tightly
    // packed BGR3 that equals image width.
    sp_p.wstride = W;
    sp_p.hstride = H;

    dp_p.fd = dst.fd;
    dp_p.fmt = rga::PixelFormat::Nv12;
    dp_p.width = W;
    dp_p.height = H;
    dp_p.wstride = W;
    dp_p.hstride = H;

    if (!rga::convert(sp_p, dp_p)) {
        fprintf(stderr, "rga-csc-probe: rga::convert FAILED for solid BGR(%u,%u,%u) at %dx%d\n", b,
                g, r, W, H);
        return false;
    }

    void* dp = ::mmap(nullptr, nv12_size, PROT_READ, MAP_SHARED, dst.fd, 0);
    if (dp == MAP_FAILED) {
        fprintf(stderr, "mmap dst: failed\n");
        return false;
    }
    const uint8_t* y = static_cast<const uint8_t*>(dp);

    uint8_t expected = expected_nv12_luma_bt601(b, g, r);
    // Sample a few pixels rather than scanning the whole plane (cheap +
    // catches "didn't convert at all" cases — all-zero or all-same-wrong).
    int samples[][2] = {
        {W / 4, H / 4}, {W / 2, H / 2}, {3 * W / 4, 3 * H / 4}, {0, 0}, {W - 1, H - 1},
    };
    bool ok = true;
    for (auto& s : samples) {
        uint8_t got = y[s[1] * W + s[0]];
        // Allow ±8 LSB tolerance — different RGA paths use slightly
        // different coefficient rounding; we just want to know it's not
        // black (0) or garbage.
        int diff = int(got) - int(expected);
        if (diff < -8 || diff > 8) {
            fprintf(stderr, "  pixel (%d,%d): got Y=%u expected ~%u (diff %d) — FAIL\n", s[0], s[1],
                    got, expected, diff);
            ok = false;
        } else {
            fprintf(stderr, "  pixel (%d,%d): got Y=%u (expected ~%u) — ok\n", s[0], s[1], got,
                    expected);
        }
    }
    ::munmap(dp, nv12_size);
    return ok;
}

} // namespace

int main() {
    const int W = 640, H = 480;
    fprintf(stderr, "rga-csc-probe: BGR3 → NV12 at %dx%d via librga\n", W, H);
    int pass = 0, fail = 0;
    struct {
        uint8_t b, g, r;
        const char* name;
    } cases[] = {
        {0xff, 0x00, 0x00, "pure blue"},
        {0x00, 0xff, 0x00, "pure green"},
        {0x00, 0x00, 0xff, "pure red"},
        {0x80, 0x80, 0x80, "mid grey"},
    };
    for (auto& c : cases) {
        fprintf(stderr, "--- case: %s ---\n", c.name);
        if (check_solid_color(W, H, c.b, c.g, c.r))
            ++pass;
        else
            ++fail;
    }
    fprintf(stderr, "rga-csc-probe: %d passed, %d failed\n", pass, fail);
    return fail == 0 ? 0 : 1;
}
