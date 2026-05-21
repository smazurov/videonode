#include "src/source/set_format_parser.hpp"

#include <gtest/gtest.h>

using source::parse_set_format;
using source::SetFormatError;
using source::SetFormatRequest;

namespace {

SetFormatRequest parse_ok(std::string_view s) {
    SetFormatRequest out;
    SetFormatError err;
    EXPECT_TRUE(parse_set_format(s, out, err)) << err.message;
    return out;
}

SetFormatError parse_fail(std::string_view s) {
    SetFormatRequest out;
    SetFormatError err;
    EXPECT_FALSE(parse_set_format(s, out, err));
    return err;
}

} // namespace

TEST(SetFormatParser, MinimalRequiredFields) {
    auto r = parse_ok(R"({"fourcc":"YUYV","w":1920,"h":1080})");
    EXPECT_EQ(r.fourcc, "YUYV");
    EXPECT_EQ(r.w, 1920u);
    EXPECT_EQ(r.h, 1080u);
    EXPECT_EQ(r.fps, 0u); // unspecified
}

TEST(SetFormatParser, WithFps) {
    auto r = parse_ok(R"({"fourcc":"NV12","w":3840,"h":2160,"fps":30})");
    EXPECT_EQ(r.fps, 30u);
}

TEST(SetFormatParser, FieldOrderIndependent) {
    auto r = parse_ok(R"({"h":1080,"fps":60,"w":1920,"fourcc":"MJPG"})");
    EXPECT_EQ(r.fourcc, "MJPG");
    EXPECT_EQ(r.w, 1920u);
    EXPECT_EQ(r.h, 1080u);
    EXPECT_EQ(r.fps, 60u);
}

TEST(SetFormatParser, UnknownKeyIgnored) {
    auto r = parse_ok(R"({"fourcc":"YUYV","w":640,"h":480,"future_field":"x"})");
    EXPECT_EQ(r.w, 640u);
}

TEST(SetFormatParser, WhitespaceTolerated) {
    auto r = parse_ok("  {\n  \"fourcc\" : \"YUYV\" ,\n  \"w\" : 1920 ,\n  \"h\" : 1080\n}  ");
    EXPECT_EQ(r.w, 1920u);
}

TEST(SetFormatParser, NotAnObject) {
    auto e = parse_fail("[]");
    EXPECT_EQ(e.code, -32602);
}

TEST(SetFormatParser, Empty) {
    auto e = parse_fail("");
    EXPECT_EQ(e.code, -32602);
}

TEST(SetFormatParser, TruncatedAfterOpenBrace) {
    auto e = parse_fail("{");
    EXPECT_EQ(e.code, -32602);
}

TEST(SetFormatParser, MissingFourcc) {
    auto e = parse_fail(R"({"w":1920,"h":1080})");
    EXPECT_EQ(e.code, -32602);
}

TEST(SetFormatParser, MissingWidth) {
    auto e = parse_fail(R"({"fourcc":"YUYV","h":1080})");
    EXPECT_EQ(e.code, -32602);
}

TEST(SetFormatParser, MissingHeight) {
    auto e = parse_fail(R"({"fourcc":"YUYV","w":1920})");
    EXPECT_EQ(e.code, -32602);
}

TEST(SetFormatParser, BadKeyQuoting) {
    auto e = parse_fail(R"({fourcc:"YUYV","w":1920,"h":1080})");
    EXPECT_EQ(e.code, -32602);
}

TEST(SetFormatParser, FourccNotString) {
    auto e = parse_fail(R"({"fourcc":42,"w":1920,"h":1080})");
    EXPECT_EQ(e.code, -32602);
}

TEST(SetFormatParser, WidthNotNumber) {
    auto e = parse_fail(R"({"fourcc":"YUYV","w":"1920","h":1080})");
    EXPECT_EQ(e.code, -32602);
}

TEST(SetFormatParser, MissingComma) {
    auto e = parse_fail(R"({"fourcc":"YUYV" "w":1920,"h":1080})");
    EXPECT_EQ(e.code, -32602);
}

TEST(SetFormatParser, ZeroWidth) {
    auto e = parse_fail(R"({"fourcc":"YUYV","w":0,"h":1080})");
    EXPECT_EQ(e.code, -32602);
}

TEST(SetFormatParser, ZeroHeight) {
    auto e = parse_fail(R"({"fourcc":"YUYV","w":1920,"h":0})");
    EXPECT_EQ(e.code, -32602);
}

TEST(SetFormatParser, WidthExceedsCap) {
    auto e = parse_fail(R"({"fourcc":"YUYV","w":20000,"h":1080})");
    EXPECT_EQ(e.code, -32602);
}

TEST(SetFormatParser, HeightExceedsCap) {
    auto e = parse_fail(R"({"fourcc":"YUYV","w":1920,"h":20000})");
    EXPECT_EQ(e.code, -32602);
}

TEST(SetFormatParser, OddWidth) {
    auto e = parse_fail(R"({"fourcc":"YUYV","w":1921,"h":1080})");
    EXPECT_EQ(e.code, -32602);
}

TEST(SetFormatParser, OddHeight) {
    auto e = parse_fail(R"({"fourcc":"YUYV","w":1920,"h":1081})");
    EXPECT_EQ(e.code, -32602);
}

TEST(SetFormatParser, FpsExceedsCap) {
    auto e = parse_fail(R"({"fourcc":"YUYV","w":1920,"h":1080,"fps":1000})");
    EXPECT_EQ(e.code, -32602);
}

TEST(SetFormatParser, IntegerOverflowInWidth) {
    // 2^32 = 4294967296 — would silently truncate to 0 if not range-checked.
    // parse_uint refuses anything beyond uint64_t; values above kMaxDim
    // are rejected on the dim check.
    auto e = parse_fail(R"({"fourcc":"YUYV","w":4294967296,"h":1080})");
    EXPECT_EQ(e.code, -32602);
}
