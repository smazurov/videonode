// Unit tests for ipc/dmabuf_header — round-trip, magic mismatch,
// truncated input, and forward-compat trailing bytes.

#include "src/ipc/dmabuf_header.hpp"

#include <gtest/gtest.h>

namespace {

dmabuf_header::Header make_nv12_single_buf() {
    dmabuf_header::Header h;
    h.slot_index = 7;
    h.width = 1920;
    h.height = 1080;
    h.format = "NV12";
    h.frame_idx = 42;
    h.color_matrix = dmabuf_header::ColorMatrix::Bt601;
    h.color_range = dmabuf_header::ColorRange::Limited;
    h.chroma_siting = dmabuf_header::ChromaSiting::Mpeg2;
    h.plane_pitches = {1920, 1920};
    h.plane_offsets = {0, 1920 * 1080};
    h.generation = 9000;
    return h;
}

} // namespace

TEST(DmabufHeader, RoundTripNV12) {
    auto h = make_nv12_single_buf();
    auto bytes = dmabuf_header::Encode(h);
    EXPECT_EQ(bytes.size(), dmabuf_header::SerializedSize(2));
    EXPECT_EQ(bytes.size(), 60u);

    dmabuf_header::Header decoded;
    std::string err;
    EXPECT_TRUE(dmabuf_header::Decode(bytes, decoded, &err)) << err;
    EXPECT_EQ(decoded.slot_index, 7u);
    EXPECT_EQ(decoded.width, 1920u);
    EXPECT_EQ(decoded.height, 1080u);
    EXPECT_EQ(decoded.format, "NV12");
    EXPECT_EQ(decoded.frame_idx, 42u);
    EXPECT_EQ(decoded.color_matrix, dmabuf_header::ColorMatrix::Bt601);
    EXPECT_EQ(decoded.color_range, dmabuf_header::ColorRange::Limited);
    EXPECT_EQ(decoded.chroma_siting, dmabuf_header::ChromaSiting::Mpeg2);
    ASSERT_EQ(decoded.plane_pitches.size(), 2u);
    EXPECT_EQ(decoded.plane_pitches[0], 1920u);
    EXPECT_EQ(decoded.plane_pitches[1], 1920u);
    ASSERT_EQ(decoded.plane_offsets.size(), 2u);
    EXPECT_EQ(decoded.plane_offsets[0], 0u);
    EXPECT_EQ(decoded.plane_offsets[1], 1920u * 1080u);
    EXPECT_EQ(decoded.generation, 9000u);
}

TEST(DmabufHeader, RoundTripSinglePlane) {
    dmabuf_header::Header h;
    h.slot_index = 0;
    h.width = 640;
    h.height = 480;
    h.format = "YUYV";
    h.plane_pitches = {640 * 2};
    h.plane_offsets = {0};
    auto bytes = dmabuf_header::Encode(h);
    EXPECT_EQ(bytes.size(), dmabuf_header::SerializedSize(1));
    EXPECT_EQ(bytes.size(), 52u);

    dmabuf_header::Header decoded;
    EXPECT_TRUE(dmabuf_header::Decode(bytes, decoded));
    EXPECT_EQ(decoded.format, "YUYV");
    EXPECT_EQ(decoded.plane_pitches.size(), 1u);
    EXPECT_EQ(decoded.plane_pitches[0], 1280u);
}

TEST(DmabufHeader, RejectsBadMagic) {
    auto h = make_nv12_single_buf();
    auto bytes = dmabuf_header::Encode(h);
    bytes[0] = 0xff;
    dmabuf_header::Header decoded;
    std::string err;
    EXPECT_FALSE(dmabuf_header::Decode(bytes, decoded, &err));
    EXPECT_FALSE(err.empty());
}

TEST(DmabufHeader, RejectsTruncatedPrefix) {
    auto h = make_nv12_single_buf();
    auto bytes = dmabuf_header::Encode(h);
    bytes.resize(20); // less than 36-byte prefix
    dmabuf_header::Header decoded;
    EXPECT_FALSE(dmabuf_header::Decode(bytes, decoded));
}

TEST(DmabufHeader, RejectsTruncatedPlaneArrays) {
    auto h = make_nv12_single_buf();
    auto bytes = dmabuf_header::Encode(h);
    bytes.resize(40); // prefix + partial plane data
    dmabuf_header::Header decoded;
    EXPECT_FALSE(dmabuf_header::Decode(bytes, decoded));
}

TEST(DmabufHeader, RejectsZeroPlaneCount) {
    auto h = make_nv12_single_buf();
    auto bytes = dmabuf_header::Encode(h);
    bytes[35] = 0; // plane_count = 0
    dmabuf_header::Header decoded;
    EXPECT_FALSE(dmabuf_header::Decode(bytes, decoded));
}

TEST(DmabufHeader, RejectsOversizedPlaneCount) {
    auto h = make_nv12_single_buf();
    auto bytes = dmabuf_header::Encode(h);
    bytes[35] = 99; // exceeds kMaxPlanes
    dmabuf_header::Header decoded;
    EXPECT_FALSE(dmabuf_header::Decode(bytes, decoded));
}

TEST(DmabufHeader, ToleratesTrailingBytes) {
    auto h = make_nv12_single_buf();
    auto bytes = dmabuf_header::Encode(h);
    // Append unknown trailing bytes — a future version might extend the
    // header; today's decoder must skip them gracefully.
    bytes.push_back(0xaa);
    bytes.push_back(0xbb);
    dmabuf_header::Header decoded;
    EXPECT_TRUE(dmabuf_header::Decode(bytes, decoded));
    EXPECT_EQ(decoded.frame_idx, 42u);
}

TEST(DmabufHeader, ShortFourccPaddedWithNul) {
    dmabuf_header::Header h;
    h.slot_index = 0;
    h.width = 64;
    h.height = 64;
    h.format = "Y8"; // 2 chars
    h.plane_pitches = {64};
    h.plane_offsets = {0};
    auto bytes = dmabuf_header::Encode(h);
    dmabuf_header::Header decoded;
    EXPECT_TRUE(dmabuf_header::Decode(bytes, decoded));
    // Trailing nuls are stripped on decode so callers comparing to "Y8"
    // see exactly that.
    EXPECT_EQ(decoded.format, "Y8");
}
