// Regression for the all-green-frame bug: TurboJPEG decodes MJPEG into
// an nv12_buf::Buffer's CPU mmap, but capture_session/orchestrator never
// unmaps the slot or calls sync_end before broadcasting. On radeonsi
// (host Fedora dev box) the dirty pages stay in the writer's GBM staging
// vma — a separate mmap of the same dma-buf fd (what videonode-sink
// does after recvmsg(SCM_RIGHTS)) reads the original zero-filled bo,
// rendering pure green NV12 downstream.
//
// This test allocates via the same nv12_buf::Allocator path used by
// capture_session, decodes a synthesized JPEG through TurboJpegDec, then
// — without unmapping or syncing — mmaps the Y dma-buf fd a second time
// and checks that at least one byte is non-zero. With the bug present
// the second mapping reads all zeros and the test fails; once
// capture_session adds an unmap or nv12_buf::sync_end(Write) before
// broadcast it should pass.

#include "src/capture/jpeg_dec_turbo.hpp"
#include "src/render/csc_placebo.hpp"
#include "src/render/nv12_buf.hpp"

#include <gtest/gtest.h>
#include <turbojpeg.h>

#include <cstdint>
#include <cstring>
#include <sys/mman.h>
#include <unistd.h>
#include <vector>

namespace {

constexpr int kW = 64;
constexpr int kH = 64;

// Column-indexed Y gradient + mid-gray chroma. After JPEG q=90 round
// trip the Y plane keeps a wide spread of non-zero values; "any non-zero
// byte" is a robust assertion that the writes are visible to a separate
// mmap reader.
std::vector<uint8_t> make_i420_pattern(int w, int h) {
    std::vector<uint8_t> i420(static_cast<size_t>(w) * h * 3 / 2);
    for (int y = 0; y < h; ++y) {
        for (int x = 0; x < w; ++x) {
            i420[static_cast<size_t>(y) * w + x] = static_cast<uint8_t>(((x + y) & 0xFF) | 0x10);
        }
    }
    uint8_t* uv = i420.data() + static_cast<size_t>(w) * h;
    std::memset(uv, 128, static_cast<size_t>(w) * h / 2);
    return i420;
}

std::vector<uint8_t> compress_i420_to_jpeg(const std::vector<uint8_t>& i420, int w, int h) {
    tjhandle h_enc = tjInitCompress();
    EXPECT_NE(h_enc, nullptr);
    unsigned char* jpeg_buf = nullptr;
    unsigned long jpeg_size = 0;
    int rc =
        tjCompressFromYUV(h_enc, i420.data(), w, 1, h, TJSAMP_420, &jpeg_buf, &jpeg_size, 90, 0);
    EXPECT_EQ(rc, 0) << tjGetErrorStr2(h_enc);
    std::vector<uint8_t> out(jpeg_buf, jpeg_buf + jpeg_size);
    tjFree(jpeg_buf);
    tjDestroy(h_enc);
    return out;
}

} // namespace

TEST(JpegDecDmabufVisibility, SeparateMmapSeesDecodedY) {
    // Need a real DRM render node for the GBM allocator. Headless CI
    // (no /dev/dri/renderD12*) silently skips, matching csc_gles tests.
    if (!csc_placebo::init()) {
        GTEST_SKIP() << "csc_gles::init failed — no DRM render node on this host";
    }
    gbm_device* dev = csc_placebo::gbm_device_for_io();
    ASSERT_NE(dev, nullptr);

    nv12_buf::Allocator alloc;
    ASSERT_TRUE(alloc.init(dev));
    nv12_buf::Buffer buf = alloc.alloc(kW, kH);
    ASSERT_TRUE(buf.valid()) << "nv12_buf::Allocator::alloc failed";

    nv12_buf::Mapping m = nv12_buf::map_rw(buf);
    ASSERT_NE(m.y, nullptr);
    ASSERT_NE(m.uv, nullptr);

    // Synthesize and decode a JPEG into the slot. This is the exact API
    // surface capture_session.cpp wires up for the TurboJPEG fallback.
    const auto i420 = make_i420_pattern(kW, kH);
    const auto jpeg = compress_i420_to_jpeg(i420, kW, kH);

    jpeg_dec::TurboJpegDec::Slot slot;
    slot.y_fd = buf.y_fd;
    slot.uv_fd = buf.uv_fd;
    slot.y_mapped = static_cast<uint8_t*>(m.y);
    slot.uv_mapped = static_cast<uint8_t*>(m.uv);
    slot.y_pitch = buf.y_pitch;
    slot.uv_pitch = buf.uv_pitch;

    jpeg_dec::TurboJpegDec dec;
    ASSERT_TRUE(dec.init(kW, kH, {slot}));

    jpeg_dec::DecodedNv12 out;
    ASSERT_TRUE(dec.decode(jpeg, out));

    // DELIBERATELY do NOT unmap and do NOT call nv12_buf::sync_end here.
    // This mirrors the current production path — capture_session holds
    // the slot's map_rw pointers for the full session lifetime, and the
    // orchestrator broadcasts the dma-buf fd to consumers immediately
    // after decode() returns. The bug is that a separate consumer that
    // mmaps the same fd reads the bo's original (zero) contents because
    // the dirty pages live in the writer's vma, not in the dma-buf.

    // Consumer-side view: fresh mmap on the Y dma-buf fd, exactly like
    // videonode-sink does after receiving the fd over SCM_RIGHTS.
    const off_t fd_size = ::lseek(buf.y_fd, 0, SEEK_END);
    ASSERT_GT(fd_size, 0) << "lseek(y_fd) failed: " << std::strerror(errno);
    void* reader =
        ::mmap(nullptr, static_cast<size_t>(fd_size), PROT_READ, MAP_SHARED, buf.y_fd, 0);
    ASSERT_NE(reader, MAP_FAILED) << "mmap of y_fd failed: " << std::strerror(errno);

    const auto* yr = static_cast<const uint8_t*>(reader);
    int nonzero = 0;
    for (int row = 0; row < kH; ++row) {
        for (int col = 0; col < kW; ++col) {
            if (yr[static_cast<size_t>(row) * buf.y_pitch + col] != 0)
                ++nonzero;
        }
    }
    ::munmap(reader, static_cast<size_t>(fd_size));
    nv12_buf::unmap(buf);

    EXPECT_GT(nonzero, 0) << "consumer's fresh mmap of NV12 dma-buf reads all zeros — the "
                             "TurboJPEG decoder's CPU writes are invisible to readers because "
                             "capture_session/orchestrator never unmaps or sync_end's the slot "
                             "before broadcasting. This is the all-green-frame bug.";
}
