#include "src/source/y4m_reader.hpp"

#include <gtest/gtest.h>

#include <fcntl.h>
#include <string>
#include <unistd.h>
#include <vector>

using source::Y4mReader;
using Result = source::Y4mReader::Result;

namespace {

constexpr const char* kHeader = "YUV4MPEG2 W4 H4 F25:1 Ip A1:1 C420mpeg2\n";
constexpr size_t kFrameBytes = 4 * 4 * 3 / 2;

std::span<const uint8_t> bytes(const std::string& s) {
    return {reinterpret_cast<const uint8_t*>(s.data()), s.size()};
}

std::string frame_block(uint8_t fill) {
    return "FRAME\n" + std::string(kFrameBytes, static_cast<char>(fill));
}

} // namespace

TEST(Y4mReader, HeaderParsesGeometryAndFps) {
    Y4mReader r;
    auto fr = r.feed(bytes(kHeader));
    EXPECT_EQ(fr.result, Result::Header);
    EXPECT_EQ(fr.consumed, std::string(kHeader).size());
    EXPECT_EQ(r.format().width, 4);
    EXPECT_EQ(r.format().height, 4);
    EXPECT_EQ(r.format().fps(), 25u);
    EXPECT_EQ(r.format().frame_bytes(), kFrameBytes);
}

TEST(Y4mReader, TwoFramesInOneSpanSurfaceOneAtATime) {
    Y4mReader r;
    const std::string stream = std::string(kHeader) + frame_block(0x11) + frame_block(0x22);
    auto all = bytes(stream);

    auto fr = r.feed(all);
    EXPECT_EQ(fr.result, Result::Header);
    all = all.subspan(fr.consumed);

    fr = r.feed(all);
    EXPECT_EQ(fr.result, Result::Frame);
    ASSERT_EQ(r.frame().size(), kFrameBytes);
    EXPECT_EQ(r.frame()[0], 0x11);
    all = all.subspan(fr.consumed);

    fr = r.feed(all);
    EXPECT_EQ(fr.result, Result::Frame);
    EXPECT_EQ(r.frame()[kFrameBytes - 1], 0x22);
    EXPECT_EQ(fr.consumed, all.size());
}

TEST(Y4mReader, FrameSplitAcrossSingleByteChunks) {
    Y4mReader r;
    const std::string stream = std::string(kHeader) + frame_block(0x33);
    int headers = 0;
    int frames = 0;
    for (char c : stream) {
        auto fr = r.feed(bytes(std::string(1, c)));
        ASSERT_NE(fr.result, Result::Error);
        EXPECT_EQ(fr.consumed, 1u);
        if (fr.result == Result::Header)
            ++headers;
        if (fr.result == Result::Frame)
            ++frames;
    }
    EXPECT_EQ(headers, 1);
    EXPECT_EQ(frames, 1);
}

TEST(Y4mReader, RejectsBadStreams) {
    struct Case {
        const char* name;
        const char* input;
    };
    const Case cases[] = {
        {.name = "missing magic", .input = "garbage W4 H4\n"},
        {.name = "missing dims", .input = "YUV4MPEG2 F25:1\n"},
        {.name = "odd dims", .input = "YUV4MPEG2 W3 H4\n"},
        {.name = "unsupported colorspace", .input = "YUV4MPEG2 W4 H4 C422\n"},
        {.name = "bad fps", .input = "YUV4MPEG2 W4 H4 Fx\n"},
    };
    for (const auto& c : cases) {
        Y4mReader r;
        auto fr = r.feed(bytes(c.input));
        EXPECT_EQ(fr.result, Result::Error) << c.name;
        EXPECT_FALSE(r.error().empty()) << c.name;
    }
}

TEST(Y4mReader, RejectsBadFrameMarker) {
    Y4mReader r;
    EXPECT_EQ(r.feed(bytes(kHeader)).result, Result::Header);
    EXPECT_EQ(r.feed(bytes("NOTFRAME\n")).result, Result::Error);
}

TEST(Y4mReader, MissingFpsTagReportsZero) {
    Y4mReader r;
    EXPECT_EQ(r.feed(bytes("YUV4MPEG2 W4 H4\n")).result, Result::Header);
    EXPECT_EQ(r.format().fps(), 0u);
}

TEST(Y4mReader, ResetParsesFreshHeader) {
    Y4mReader r;
    EXPECT_EQ(r.feed(bytes(kHeader)).result, Result::Header);
    r.reset();
    EXPECT_EQ(r.feed(bytes("YUV4MPEG2 W6 H2 F30:1\n")).result, Result::Header);
    EXPECT_EQ(r.format().width, 6);
    EXPECT_EQ(r.format().height, 2);
}

TEST(Y4mReader, ConsumeFdNonBlockingLifecycle) {
    int fds[2];
    ASSERT_EQ(::pipe2(fds, O_NONBLOCK), 0);
    Y4mReader r;

    EXPECT_EQ(r.consume_fd(fds[0]), Result::NeedMore);

    const std::string head_and_frame = std::string(kHeader) + frame_block(0x44);
    ASSERT_EQ(::write(fds[1], head_and_frame.data(), head_and_frame.size()),
              ssize_t(head_and_frame.size()));
    EXPECT_EQ(r.consume_fd(fds[0]), Result::Header);
    EXPECT_EQ(r.consume_fd(fds[0]), Result::Frame);
    EXPECT_EQ(r.consume_fd(fds[0]), Result::NeedMore);

    const std::string partial = "FRAME\n\x55\x55";
    ASSERT_EQ(::write(fds[1], partial.data(), partial.size()), ssize_t(partial.size()));
    EXPECT_EQ(r.consume_fd(fds[0]), Result::NeedMore);
    ::close(fds[1]);
    EXPECT_EQ(r.consume_fd(fds[0]), Result::Eof);
    ::close(fds[0]);
}
