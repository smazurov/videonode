#include "src/source/y4m_reader.hpp"

#include <algorithm>
#include <cerrno>
#include <cstring>
#include <unistd.h>

namespace source {

namespace {

constexpr size_t kMaxLineBytes = 1024;
constexpr int kMaxDim = 16384;

bool parse_int(std::string_view s, int& out) {
    if (s.empty() || s.size() > 9)
        return false;
    int v = 0;
    for (char c : s) {
        if (c < '0' || c > '9')
            return false;
        v = v * 10 + (c - '0');
    }
    out = v;
    return true;
}

} // namespace

uint32_t Y4mFormat::fps() const {
    if (fps_num <= 0 || fps_den <= 0)
        return 0;
    return static_cast<uint32_t>((fps_num + fps_den / 2) / fps_den);
}

size_t Y4mFormat::frame_bytes() const {
    return size_t(width) * size_t(height) * 3 / 2;
}

Y4mReader::Result Y4mReader::fail(const std::string& why) {
    error_ = why;
    state_ = State::Failed;
    return Result::Error;
}

bool Y4mReader::apply_header_tag(const std::string& tag) {
    if (tag.empty())
        return true;
    const std::string_view value = std::string_view(tag).substr(1);
    switch (tag[0]) {
    case 'W':
        return parse_int(value, fmt_.width);
    case 'H':
        return parse_int(value, fmt_.height);
    case 'F': {
        const size_t colon = value.find(':');
        if (colon == std::string_view::npos)
            return false;
        return parse_int(value.substr(0, colon), fmt_.fps_num) &&
               parse_int(value.substr(colon + 1), fmt_.fps_den);
    }
    case 'C':
        return value.substr(0, 3) == "420";
    default:
        return true;
    }
}

Y4mReader::Result Y4mReader::parse_header_line() {
    if (line_.rfind("YUV4MPEG2", 0) != 0)
        return fail("missing YUV4MPEG2 magic");
    size_t pos = line_.find(' ');
    while (pos != std::string::npos) {
        const size_t start = pos + 1;
        pos = line_.find(' ', start);
        const std::string tag = line_.substr(start, pos == std::string::npos ? pos : pos - start);
        if (!apply_header_tag(tag))
            return fail("bad y4m header tag: " + tag);
    }
    if (fmt_.width <= 0 || fmt_.height <= 0)
        return fail("y4m header missing W/H");
    if (fmt_.width > kMaxDim || fmt_.height > kMaxDim)
        return fail("y4m dimensions exceed 16384");
    if ((fmt_.width & 1) != 0 || (fmt_.height & 1) != 0)
        return fail("y4m dimensions must be even");
    frame_.assign(fmt_.frame_bytes(), 0);
    line_.clear();
    state_ = State::Marker;
    return Result::Header;
}

Y4mReader::Result Y4mReader::on_line_byte(uint8_t b) {
    if (b != '\n') {
        if (line_.size() >= kMaxLineBytes)
            return fail("y4m line too long");
        line_.push_back(static_cast<char>(b));
        return Result::NeedMore;
    }
    if (state_ == State::Header)
        return parse_header_line();
    if (line_.rfind("FRAME", 0) != 0)
        return fail("missing FRAME marker");
    line_.clear();
    filled_ = 0;
    state_ = State::Body;
    return Result::NeedMore;
}

Y4mReader::FeedResult Y4mReader::feed(std::span<const uint8_t> bytes) {
    size_t consumed = 0;
    while (consumed < bytes.size()) {
        if (state_ == State::Failed)
            return {.result = Result::Error, .consumed = consumed};
        if (state_ == State::Body) {
            const size_t want = fmt_.frame_bytes() - filled_;
            const size_t take = std::min(want, bytes.size() - consumed);
            auto src = bytes.subspan(consumed, take);
            std::copy(src.begin(), src.end(), frame_.begin() + static_cast<ptrdiff_t>(filled_));
            filled_ += take;
            consumed += take;
            if (filled_ == fmt_.frame_bytes()) {
                state_ = State::Marker;
                line_.clear();
                return {.result = Result::Frame, .consumed = consumed};
            }
            continue;
        }
        const Result r = on_line_byte(bytes[consumed]);
        ++consumed;
        if (r != Result::NeedMore)
            return {.result = r, .consumed = consumed};
    }
    return {.result = Result::NeedMore, .consumed = consumed};
}

Y4mReader::Result Y4mReader::consume_fd(int fd) {
    for (;;) {
        if (state_ == State::Failed)
            return Result::Error;
        ssize_t n = 0;
        if (state_ == State::Body) {
            auto rem = std::span<uint8_t>(frame_).subspan(filled_);
            n = ::read(fd, rem.data(), rem.size());
            if (n > 0) {
                filled_ += static_cast<size_t>(n);
                if (filled_ == fmt_.frame_bytes()) {
                    state_ = State::Marker;
                    line_.clear();
                    return Result::Frame;
                }
                continue;
            }
        } else {
            uint8_t b = 0;
            n = ::read(fd, &b, 1);
            if (n > 0) {
                const Result r = on_line_byte(b);
                if (r != Result::NeedMore)
                    return r;
                continue;
            }
        }
        if (n == 0)
            return Result::Eof;
        if (errno == EINTR)
            continue;
        if (errno == EAGAIN || errno == EWOULDBLOCK)
            return Result::NeedMore;
        return fail(std::string("read: ") + strerror(errno));
    }
}

void Y4mReader::reset() {
    state_ = State::Header;
    fmt_ = {};
    line_.clear();
    filled_ = 0;
    error_.clear();
}

} // namespace source
