#include "src/sink/nut_writer.hpp"

#include <gtest/gtest.h>

#include <algorithm>
#include <string>
#include <vector>

namespace {

std::vector<uint8_t> bytes_of(const std::string& s) {
    return {s.begin(), s.end()};
}

struct Capture {
    std::vector<uint8_t> out;
    nut::ByteSink sink() {
        return [this](std::span<const uint8_t> b) {
            out.insert(out.end(), b.begin(), b.end());
            return true;
        };
    }
};

bool contains(const std::vector<uint8_t>& hay, const std::vector<uint8_t>& needle) {
    return std::search(hay.begin(), hay.end(), needle.begin(), needle.end()) != hay.end();
}

} // namespace

TEST(NutWriter, Crc32MatchesPosixWithoutFinalXor) {
    EXPECT_EQ(nut::crc32(0, bytes_of("123456789")), 0x89A1897FU);
}

TEST(NutWriter, CrcOfDataPlusBigEndianCrcIsZero) {
    auto data = bytes_of("videonode");
    const uint32_t c = nut::crc32(0, data);
    data.push_back(static_cast<uint8_t>(c >> 24));
    data.push_back(static_cast<uint8_t>(c >> 16));
    data.push_back(static_cast<uint8_t>(c >> 8));
    data.push_back(static_cast<uint8_t>(c));
    EXPECT_EQ(nut::crc32(0, data), 0U);
}

TEST(NutWriter, VarlenIsBigEndianBase128) {
    std::vector<uint8_t> v;
    nut::put_v(v, 0);
    nut::put_v(v, 127);
    nut::put_v(v, 128);
    nut::put_v(v, 16384);
    EXPECT_EQ(v, (std::vector<uint8_t>{0x00, 0x7F, 0x81, 0x00, 0x81, 0x80, 0x00}));
}

TEST(NutWriter, SignedVarlenZigZag) {
    std::vector<uint8_t> v;
    nut::put_s(v, 0);
    nut::put_s(v, 1);
    nut::put_s(v, -1);
    nut::put_s(v, 2);
    EXPECT_EQ(v, (std::vector<uint8_t>{0, 1, 2, 3}));
}

TEST(NutWriter, HeaderCarriesIdStreamsAndFourccs) {
    Capture cap;
    nut::Writer w(cap.sink());
    const std::vector<nut::AudioParams> audio{{.sample_rate = 48000, .channels = 2}};
    ASSERT_TRUE(w.write_header({.width = 64, .height = 36, .fps = 30}, audio));
    const std::string id = "nut/multimedia container";
    ASSERT_GE(cap.out.size(), id.size() + 1);
    EXPECT_EQ(std::string(cap.out.begin(), cap.out.begin() + static_cast<long>(id.size())), id);
    EXPECT_EQ(cap.out[id.size()], 0);
    EXPECT_TRUE(contains(cap.out, bytes_of("NV12")));
    EXPECT_TRUE(contains(cap.out, {'P', 'S', 'D', 16}));
    EXPECT_TRUE(contains(cap.out, bytes_of("r_frame_rate")));
}

TEST(NutWriter, RejectsFramesBeforeHeaderAndNegativePts) {
    Capture cap;
    nut::Writer w(cap.sink());
    const std::vector<uint8_t> data(16, 1);
    EXPECT_FALSE(w.write_frame(0, 0, data));
    ASSERT_TRUE(w.write_header({.width = 4, .height = 2, .fps = 30}, {}));
    EXPECT_FALSE(w.write_frame(0, -1, data));
    EXPECT_FALSE(w.write_frame(1, 0, data));
    EXPECT_TRUE(w.write_frame(0, 0, data));
}

TEST(NutWriter, OnlyFirstFrameGetsSyncpoint) {
    Capture cap;
    nut::Writer w(cap.sink());
    ASSERT_TRUE(w.write_header({.width = 4, .height = 2, .fps = 30}, {}));
    const std::vector<uint8_t> syncpoint{0x4E, 0x4B, 0xE4, 0xAD, 0xEE, 0xCA, 0x45, 0x69};
    const std::vector<uint8_t> data(12, 7);
    ASSERT_TRUE(w.write_frame(0, 0, data));
    const size_t after_first = cap.out.size();
    EXPECT_TRUE(contains(cap.out, syncpoint));
    ASSERT_TRUE(w.write_frame(0, 1, data));
    const std::vector<uint8_t> second(cap.out.begin() + static_cast<long>(after_first),
                                      cap.out.end());
    EXPECT_FALSE(contains(second, syncpoint));
}

TEST(NutWriter, RejectsInvalidParams) {
    Capture cap;
    nut::Writer w(cap.sink());
    EXPECT_FALSE(w.write_header({.width = 0, .height = 2, .fps = 30}, {}));
    const std::vector<nut::AudioParams> bad{{.sample_rate = 0, .channels = 2}};
    EXPECT_FALSE(w.write_header({.width = 4, .height = 2, .fps = 30}, bad));
}
