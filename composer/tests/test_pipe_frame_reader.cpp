// Y4mReader against real scripted children spawned through
// child_process::spawn_shell_group — the exact wiring pipe-mode sources use.

#include "src/process/child_process.hpp"
#include "src/source/y4m_reader.hpp"

#include <gtest/gtest.h>

#include <csignal>
#include <string>
#include <vector>

using source::Y4mReader;
using Result = source::Y4mReader::Result;

namespace {

constexpr const char* kEmitHeader = "printf 'YUV4MPEG2 W4 H4 F25:1\\n'";
constexpr const char* kEmitFrame = "printf 'FRAME\\n'; head -c 24 /dev/zero";

std::vector<Result> drain(int fd) {
    Y4mReader r;
    std::vector<Result> events;
    for (;;) {
        Result res = r.consume_fd(fd);
        events.push_back(res);
        if (res == Result::Eof || res == Result::Error)
            return events;
    }
}

} // namespace

TEST(PipeFrameReader, ExactFramesThenEof) {
    const std::string cmd = std::string(kEmitHeader) + "; " + kEmitFrame + "; " + kEmitFrame;
    auto r = child_process::spawn_shell_group(cmd, SIGKILL);
    ASSERT_GT(r.pid, 0);
    ASSERT_GE(r.stdout_fd, 0);

    auto events = drain(r.stdout_fd);
    const std::vector<Result> want = {Result::Header, Result::Frame, Result::Frame, Result::Eof};
    EXPECT_EQ(events, want);
    child_process::reap_group(r.pid, r.stdout_fd, 1000);
}

TEST(PipeFrameReader, ShortFinalFrameSurfacesEof) {
    const std::string cmd = std::string(kEmitHeader) + "; printf 'FRAME\\n'; head -c 12 /dev/zero";
    auto r = child_process::spawn_shell_group(cmd, SIGKILL);
    ASSERT_GT(r.pid, 0);

    auto events = drain(r.stdout_fd);
    const std::vector<Result> want = {Result::Header, Result::Eof};
    EXPECT_EQ(events, want);
    child_process::reap_group(r.pid, r.stdout_fd, 1000);
}

TEST(PipeFrameReader, ImmediateEof) {
    auto r = child_process::spawn_shell_group("true", SIGKILL);
    ASSERT_GT(r.pid, 0);

    auto events = drain(r.stdout_fd);
    const std::vector<Result> want = {Result::Eof};
    EXPECT_EQ(events, want);
    child_process::reap_group(r.pid, r.stdout_fd, 1000);
}

TEST(PipeFrameReader, GarbageStreamSurfacesError) {
    auto r = child_process::spawn_shell_group("printf 'not a y4m stream\\n'", SIGKILL);
    ASSERT_GT(r.pid, 0);

    auto events = drain(r.stdout_fd);
    ASSERT_FALSE(events.empty());
    EXPECT_EQ(events.back(), Result::Error);
    child_process::reap_group(r.pid, r.stdout_fd, 1000);
}
