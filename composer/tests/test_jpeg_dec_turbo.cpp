// Unit tests for capture/jpeg_dec_turbo — verifies the decoder honors
// the caller-supplied per-slot stride. This is the contract the
// cross-process EGL importer (videonode-composer) relies on: producer-
// reported pitch must match the byte layout inside the BO.

#include "src/capture/jpeg_dec_turbo.hpp"

#include <gtest/gtest.h>
#include <turbojpeg.h>

#include <cstdint>
#include <cstring>
#include <span>
#include <vector>

namespace {

constexpr int kW = 64;
constexpr int kH = 64;

// Synthesize a deterministic NV12 source frame. Y plane is a
// horizontal gradient (col index); UV plane is a fixed mid-gray
// (Cb=128, Cr=128) so chroma comparisons stay stable across the
// 4:2:0 round-trip.
struct NV12Frame {
    std::vector<uint8_t> y;
    std::vector<uint8_t> uv;
};

NV12Frame make_nv12_pattern(int w, int h) {
    NV12Frame f;
    f.y.resize(static_cast<size_t>(w) * h);
    f.uv.resize(static_cast<size_t>(w) * h / 2);
    for (int row = 0; row < h; ++row) {
        for (int col = 0; col < w; ++col) {
            f.y[row * w + col] = static_cast<uint8_t>(col & 0xFF);
        }
    }
    for (auto& b : f.uv)
        b = 128;
    return f;
}

// Compress an NV12 frame to a baseline 4:2:0 JPEG using libjpeg-turbo's
// planar-YUV input path. Splits the NV12 UV plane into separate U/V
// planes since tjCompressFromYUV expects I420.
std::vector<uint8_t> compress_nv12_to_jpeg(const NV12Frame& f, int w, int h) {
    const size_t y_size = static_cast<size_t>(w) * h;
    const size_t chroma_samples = y_size / 4;
    std::vector<uint8_t> i420(y_size * 3 / 2);
    std::memcpy(i420.data(), f.y.data(), f.y.size());
    const std::span<uint8_t> i420_span{i420};
    const std::span<uint8_t> up = i420_span.subspan(y_size, chroma_samples);
    const std::span<uint8_t> vp = i420_span.subspan(y_size + chroma_samples, chroma_samples);
    for (size_t i = 0; i < f.uv.size() / 2; ++i) {
        up[i] = f.uv[2 * i];
        vp[i] = f.uv[2 * i + 1];
    }
    tjhandle h_enc = tjInitCompress();
    EXPECT_NE(h_enc, nullptr);
    unsigned char* jpeg_buf = nullptr;
    unsigned long jpeg_size = 0;
    int rc =
        tjCompressFromYUV(h_enc, i420.data(), w, 1, h, TJSAMP_420, &jpeg_buf, &jpeg_size, 90, 0);
    EXPECT_EQ(rc, 0) << tjGetErrorStr2(h_enc);
    const std::span<const uint8_t> jpeg_span{jpeg_buf, jpeg_size};
    std::vector<uint8_t> out(jpeg_span.begin(), jpeg_span.end());
    tjFree(jpeg_buf);
    tjDestroy(h_enc);
    return out;
}

} // namespace

TEST(TurboJpegDec, PreservesPaddedSlotStride) {
    const NV12Frame src = make_nv12_pattern(kW, kH);
    const std::vector<uint8_t> jpeg = compress_nv12_to_jpeg(src, kW, kH);

    constexpr uint32_t kYPitch = 128;  // > width, simulates GBM R8 BO stride
    constexpr uint32_t kUVPitch = 128; // > width, simulates GBM GR88 BO stride
    std::vector<uint8_t> y_buf(static_cast<size_t>(kYPitch) * kH, 0xAA);
    std::vector<uint8_t> uv_buf(static_cast<size_t>(kUVPitch) * (kH / 2), 0xBB);

    jpeg_dec::TurboJpegDec::Slot slot;
    slot.y_fd = 100; // sentinel — decoder forwards, never touches
    slot.uv_fd = 101;
    slot.y_mapped = y_buf.data();
    slot.uv_mapped = uv_buf.data();
    slot.y_pitch = kYPitch;
    slot.uv_pitch = kUVPitch;

    jpeg_dec::TurboJpegDec dec;
    ASSERT_TRUE(dec.init(kW, kH, {slot}));

    jpeg_dec::DecodedNv12 out;
    ASSERT_TRUE(dec.decode(jpeg, out));

    EXPECT_EQ(out.fd, 100);
    EXPECT_EQ(out.plane1_fd, 101);
    EXPECT_EQ(out.width, kW);
    EXPECT_EQ(out.height, kH);
    EXPECT_EQ(out.y_pitch, kYPitch);
    EXPECT_EQ(out.uv_pitch, kUVPitch);
    EXPECT_EQ(out.y_offset, 0u);
    EXPECT_EQ(out.uv_offset, 0u); // split slot

    // Each Y row must land at row*kYPitch in the BO, not row*kW. Sample
    // a few rows. The gradient is col-indexed, so column 5 of row N has
    // value 5; if the decoder wrote at width-stride, row 2 column 5
    // would be at offset 2*kW + 5 = 133, but the BO expects 2*128 + 5 =
    // 261. We check both: the padded-stride offset has the right value
    // AND the bytes past the row's content are the original 0xAA fill.
    for (int row : {1, 5, 30, 63}) {
        EXPECT_EQ(y_buf[static_cast<size_t>(row) * kYPitch + 5], 5)
            << "row " << row << " col 5 not at padded stride";
        // tail padding past col=kW should still be the 0xAA fill — JPEG
        // doesn't write past the column boundary.
        EXPECT_EQ(y_buf[static_cast<size_t>(row) * kYPitch + kW + 1], 0xAA)
            << "row " << row << " tail overwritten";
    }

    // UV plane: mid-gray. After 4:2:0 JPEG round-trip at q=90, values
    // stay close to 128. Each chroma row must be at row*kUVPitch.
    for (int row : {1, 10, 31}) {
        const uint8_t cb = uv_buf[static_cast<size_t>(row) * kUVPitch + 0];
        const uint8_t cr = uv_buf[static_cast<size_t>(row) * kUVPitch + 1];
        EXPECT_NEAR(cb, 128, 4) << "row " << row << " Cb wrong at padded uv stride";
        EXPECT_NEAR(cr, 128, 4) << "row " << row << " Cr wrong at padded uv stride";
        // Tail padding past UV content (kW bytes = kW/2 samples * 2 bytes)
        // must remain the 0xBB fill.
        EXPECT_EQ(uv_buf[static_cast<size_t>(row) * kUVPitch + kW + 1], 0xBB)
            << "row " << row << " UV tail overwritten";
    }
}

TEST(TurboJpegDec, TightStrideStillWorks) {
    // Rig dma_heap path: y_pitch == uv_pitch == width, single contiguous
    // fd. Confirms the fix doesn't break the existing happy path.
    const NV12Frame src = make_nv12_pattern(kW, kH);
    const std::vector<uint8_t> jpeg = compress_nv12_to_jpeg(src, kW, kH);

    std::vector<uint8_t> bo(static_cast<size_t>(kW) * kH * 3 / 2, 0);

    jpeg_dec::TurboJpegDec::Slot slot;
    slot.y_fd = 42;
    slot.uv_fd = 42; // contiguous: same fd
    slot.y_mapped = bo.data();
    slot.uv_mapped = std::span<uint8_t>{bo}.subspan(static_cast<size_t>(kW) * kH).data();
    slot.y_pitch = kW;
    slot.uv_pitch = kW;

    jpeg_dec::TurboJpegDec dec;
    ASSERT_TRUE(dec.init(kW, kH, {slot}));

    jpeg_dec::DecodedNv12 out;
    ASSERT_TRUE(dec.decode(jpeg, out));

    EXPECT_EQ(out.fd, 42);
    EXPECT_EQ(out.plane1_fd, -1); // contiguous → -1 sentinel
    EXPECT_EQ(out.y_pitch, static_cast<uint32_t>(kW));
    EXPECT_EQ(out.uv_pitch, static_cast<uint32_t>(kW));
    EXPECT_EQ(out.y_offset, 0u);
    EXPECT_EQ(out.uv_offset, static_cast<uint32_t>(kW) * kH);

    // Spot-check: Y gradient lands tight.
    EXPECT_EQ(bo[0 * kW + 7], 7);
    EXPECT_EQ(bo[10 * kW + 33], 33);
}
