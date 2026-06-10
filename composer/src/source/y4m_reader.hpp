#pragma once

#include <cstddef>
#include <cstdint>
#include <span>
#include <string>
#include <vector>

namespace source {

struct Y4mFormat {
    int width = 0;
    int height = 0;
    int fps_num = 0;
    int fps_den = 1;

    [[nodiscard]] uint32_t fps() const;
    [[nodiscard]] size_t frame_bytes() const;
};

// Incremental yuv4mpegpipe parser: one stream header, then FRAME-marked
// tight I420 payloads. Accepts only the C420 colorspace family.
class Y4mReader {
  public:
    enum class Result { NeedMore, Header, Frame, Eof, Error };

    struct FeedResult {
        Result result = Result::NeedMore;
        size_t consumed = 0;
    };

    [[nodiscard]] FeedResult feed(std::span<const uint8_t> bytes);
    [[nodiscard]] Result consume_fd(int fd);

    [[nodiscard]] const Y4mFormat& format() const { return fmt_; }
    [[nodiscard]] std::span<const uint8_t> frame() const { return frame_; }
    [[nodiscard]] const std::string& error() const { return error_; }
    void reset();

  private:
    enum class State { Header, Marker, Body, Failed };

    [[nodiscard]] Result on_line_byte(uint8_t b);
    [[nodiscard]] Result parse_header_line();
    [[nodiscard]] bool apply_header_tag(const std::string& tag);
    [[nodiscard]] Result fail(const std::string& why);

    State state_ = State::Header;
    Y4mFormat fmt_;
    std::string line_;
    std::vector<uint8_t> frame_;
    size_t filled_ = 0;
    std::string error_;
};

} // namespace source
