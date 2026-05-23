// Tests for Y4mWriter — header bytes, NV12 → I420 chroma deinterleave, and
// partial-write resilience via a fake write syscall.

#include "src/process/y4m_writer.hpp"

#include <gtest/gtest.h>

#include <cerrno>
#include <cstdint>
#include <cstring>
#include <string>
#include <vector>

namespace {

// Test seam state. The WriteFn we hand to Y4mWriter is a plain function
// pointer (no captures), so we route through a singleton-ish struct.
struct FakeSink {
    std::vector<uint8_t> bytes;
    // If non-empty, each call to write returns at most chunk_sizes.front()
    // bytes (and pops it). When the list is exhausted, falls back to "write
    // everything we're given".
    std::vector<size_t> chunk_sizes;
    // If >0, the next `eintr_left` calls return -1/EINTR before any progress.
    int eintr_left = 0;

    void reset() {
        bytes.clear();
        chunk_sizes.clear();
        eintr_left = 0;
    }
};

FakeSink& sink() {
    static FakeSink s;
    return s;
}

ssize_t fake_write(int /*fd*/, const void* buf, size_t len) {
    auto& s = sink();
    if (s.eintr_left > 0) {
        --s.eintr_left;
        errno = EINTR;
        return -1;
    }
    size_t take = len;
    if (!s.chunk_sizes.empty()) {
        take = s.chunk_sizes.front();
        s.chunk_sizes.erase(s.chunk_sizes.begin());
        if (take > len)
            take = len;
    }
    const auto* p = static_cast<const uint8_t*>(buf);
    s.bytes.insert(s.bytes.end(), p, p + take);
    return static_cast<ssize_t>(take);
}

ssize_t failing_write(int /*fd*/, const void* /*buf*/, size_t /*len*/) {
    errno = EPIPE;
    return -1;
}

ssize_t eof_write(int /*fd*/, const void* /*buf*/, size_t /*len*/) {
    return 0;
}

vn::process::Y4mWriter make_writer(int w, int h, int fps_num, int fps_den) {
    sink().reset();
    vn::process::Y4mWriter wr(/*out_fd*/ -1, w, h, fps_num, fps_den);
    wr.SetWriteFnForTest(&fake_write);
    return wr;
}

} // namespace

TEST(Y4mWriter, HeaderBytes720p60) {
    auto wr = make_writer(1280, 720, 60, 1);
    ASSERT_TRUE(wr.WriteHeader());
    std::string got(sink().bytes.begin(), sink().bytes.end());
    EXPECT_EQ(got, "YUV4MPEG2 W1280 H720 F60:1 Ip A1:1 C420\n");
}

TEST(Y4mWriter, HeaderBytes1080p60) {
    auto wr = make_writer(1920, 1080, 60, 1);
    ASSERT_TRUE(wr.WriteHeader());
    std::string got(sink().bytes.begin(), sink().bytes.end());
    EXPECT_EQ(got, "YUV4MPEG2 W1920 H1080 F60:1 Ip A1:1 C420\n");
}

TEST(Y4mWriter, HeaderOddRatio) {
    // Sanity: 30000:1001 is the canonical NTSC framerate; verify both
    // numerator and denominator survive printf without truncation.
    auto wr = make_writer(640, 480, 30000, 1001);
    ASSERT_TRUE(wr.WriteHeader());
    std::string got(sink().bytes.begin(), sink().bytes.end());
    EXPECT_EQ(got, "YUV4MPEG2 W640 H480 F30000:1001 Ip A1:1 C420\n");
}

TEST(Y4mWriter, FrameNV12RoundTrip4x4) {
    // Build a tiny 4x4 NV12 buffer:
    //   Y (16 bytes): 0..15
    //   UV (8 bytes, 2 rows × 4 cols = U0V0U1V1 per row): U0=20,V0=120,
    //                                                    U1=21,V1=121, ...
    // Tight strides (y_stride=4, uv_stride=4).
    constexpr int kW = 4;
    constexpr int kH = 4;
    std::vector<uint8_t> y(kW * kH);
    for (int i = 0; i < kW * kH; ++i)
        y[i] = static_cast<uint8_t>(i);

    std::vector<uint8_t> uv(kW * (kH / 2));
    // Row 0: U0V0 U1V1
    uv[0] = 20;
    uv[1] = 120;
    uv[2] = 21;
    uv[3] = 121;
    // Row 1: U2V2 U3V3
    uv[4] = 22;
    uv[5] = 122;
    uv[6] = 23;
    uv[7] = 123;

    auto wr = make_writer(kW, kH, 60, 1);
    ASSERT_TRUE(
        wr.WriteFrameNV12(std::span<const uint8_t>(y), kW, std::span<const uint8_t>(uv), kW));
    const auto& b = sink().bytes;

    // Layout: "FRAME\n" (6) + 16 Y + 4 U + 4 V = 30 bytes.
    ASSERT_EQ(b.size(), 6u + 16 + 4 + 4);
    std::string tag(b.begin(), b.begin() + 6);
    EXPECT_EQ(tag, "FRAME\n");

    // Y unchanged.
    for (int i = 0; i < 16; ++i) {
        EXPECT_EQ(b[6 + i], static_cast<uint8_t>(i)) << "Y[" << i << "]";
    }

    // U plane: 20, 21, 22, 23.
    EXPECT_EQ(b[22], 20);
    EXPECT_EQ(b[23], 21);
    EXPECT_EQ(b[24], 22);
    EXPECT_EQ(b[25], 23);

    // V plane: 120, 121, 122, 123.
    EXPECT_EQ(b[26], 120);
    EXPECT_EQ(b[27], 121);
    EXPECT_EQ(b[28], 122);
    EXPECT_EQ(b[29], 123);
}

TEST(Y4mWriter, FrameNV12HonorsRowPadding) {
    // y_stride=6 (2 bytes of right padding per row); uv_stride=6.
    constexpr int kW = 4;
    constexpr int kH = 4;
    constexpr size_t kYStride = 6;
    constexpr size_t kUVStride = 6;
    std::vector<uint8_t> y(kYStride * kH, 0xEE); // padding sentinel
    for (int row = 0; row < kH; ++row) {
        for (int col = 0; col < kW; ++col) {
            y[row * kYStride + col] = static_cast<uint8_t>(row * kW + col);
        }
    }
    std::vector<uint8_t> uv(kUVStride * (kH / 2), 0xEE);
    for (int row = 0; row < kH / 2; ++row) {
        uv[row * kUVStride + 0] = static_cast<uint8_t>(20 + row * 2);
        uv[row * kUVStride + 1] = static_cast<uint8_t>(120 + row * 2);
        uv[row * kUVStride + 2] = static_cast<uint8_t>(21 + row * 2);
        uv[row * kUVStride + 3] = static_cast<uint8_t>(121 + row * 2);
    }

    auto wr = make_writer(kW, kH, 60, 1);
    ASSERT_TRUE(wr.WriteFrameNV12(std::span<const uint8_t>(y), kYStride,
                                  std::span<const uint8_t>(uv), kUVStride));
    const auto& b = sink().bytes;
    ASSERT_EQ(b.size(), 6u + 16 + 4 + 4);

    // Padding sentinel must not leak into output.
    for (size_t i = 6; i < b.size(); ++i) {
        EXPECT_NE(b[i], 0xEE) << "leaked padding byte at output[" << i << "]";
    }
    // Spot-check Y row 1 came out as 4..7 (not pulled from padded offsets).
    EXPECT_EQ(b[6 + 4], 4);
    EXPECT_EQ(b[6 + 5], 5);
    EXPECT_EQ(b[6 + 6], 6);
    EXPECT_EQ(b[6 + 7], 7);
}

TEST(Y4mWriter, PartialWriteRetries) {
    // Force the fake to dribble out 3 bytes at a time. The writer should
    // loop until the whole header is delivered.
    auto wr = make_writer(1280, 720, 60, 1);
    sink().chunk_sizes = {3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 100};
    ASSERT_TRUE(wr.WriteHeader());
    std::string got(sink().bytes.begin(), sink().bytes.end());
    EXPECT_EQ(got, "YUV4MPEG2 W1280 H720 F60:1 Ip A1:1 C420\n");
}

TEST(Y4mWriter, EintrIsRetried) {
    auto wr = make_writer(1280, 720, 60, 1);
    sink().eintr_left = 3;
    ASSERT_TRUE(wr.WriteHeader());
    std::string got(sink().bytes.begin(), sink().bytes.end());
    EXPECT_EQ(got, "YUV4MPEG2 W1280 H720 F60:1 Ip A1:1 C420\n");
}

TEST(Y4mWriter, WriteFailurePropagates) {
    vn::process::Y4mWriter wr(/*out_fd*/ -1, 1280, 720, 60, 1);
    wr.SetWriteFnForTest(&failing_write);
    EXPECT_FALSE(wr.WriteHeader());
}

TEST(Y4mWriter, WriteEofPropagates) {
    vn::process::Y4mWriter wr(/*out_fd*/ -1, 1280, 720, 60, 1);
    wr.SetWriteFnForTest(&eof_write);
    EXPECT_FALSE(wr.WriteHeader());
}

TEST(Y4mWriter, RejectsStrideBelowWidth) {
    auto wr = make_writer(4, 4, 60, 1);
    std::vector<uint8_t> y(16, 0);
    std::vector<uint8_t> uv(8, 0);
    // y_stride=3 < width=4 → reject without writing FRAME tag.
    EXPECT_FALSE(
        wr.WriteFrameNV12(std::span<const uint8_t>(y), 3, std::span<const uint8_t>(uv), 4));
    EXPECT_TRUE(sink().bytes.empty());
}
