// Tests for the composer background hex-color parser.

#include "src/render/hex_color.hpp"

#include <gtest/gtest.h>

#include <cstdint>

namespace {

TEST(HexColor, SixDigitGetsOpaqueAlpha) {
    uint32_t out = 0;
    ASSERT_TRUE(hex_color::parse("#1a2b3c", out));
    EXPECT_EQ(out, 0x1A2B3CFFU);
}

TEST(HexColor, LeadingHashIsOptional) {
    uint32_t out = 0;
    ASSERT_TRUE(hex_color::parse("1a2b3c", out));
    EXPECT_EQ(out, 0x1A2B3CFFU);
}

TEST(HexColor, EightDigitKeepsExplicitAlpha) {
    uint32_t out = 0;
    ASSERT_TRUE(hex_color::parse("#1a2b3c80", out));
    EXPECT_EQ(out, 0x1A2B3C80U);
}

TEST(HexColor, UppercaseDigits) {
    uint32_t out = 0;
    ASSERT_TRUE(hex_color::parse("#FFAA00", out));
    EXPECT_EQ(out, 0xFFAA00FFU);
}

TEST(HexColor, PureBlackAndWhite) {
    uint32_t black = 0xDEADBEEFU;
    ASSERT_TRUE(hex_color::parse("#000000", black));
    EXPECT_EQ(black, 0x000000FFU);

    uint32_t white = 0;
    ASSERT_TRUE(hex_color::parse("ffffffff", white));
    EXPECT_EQ(white, 0xFFFFFFFFU);
}

TEST(HexColor, EmptyLeavesOutUntouched) {
    uint32_t out = 0x12345678U;
    ASSERT_TRUE(hex_color::parse("", out));
    EXPECT_EQ(out, 0x12345678U);
    ASSERT_TRUE(hex_color::parse("#", out));
    EXPECT_EQ(out, 0x12345678U);
}

TEST(HexColor, RejectsBadLength) {
    uint32_t out = 0x12345678U;
    EXPECT_FALSE(hex_color::parse("#1a2", out));
    EXPECT_FALSE(hex_color::parse("12345", out));
    EXPECT_FALSE(hex_color::parse("1234567", out));
    EXPECT_EQ(out, 0x12345678U);
}

TEST(HexColor, RejectsNonHexDigit) {
    uint32_t out = 0x12345678U;
    EXPECT_FALSE(hex_color::parse("#12345g", out));
    EXPECT_FALSE(hex_color::parse("zzzzzz", out));
    EXPECT_EQ(out, 0x12345678U);
}

} // namespace
