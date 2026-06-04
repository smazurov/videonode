// Tests for child_process — uses small `cat`/`echo` subprocesses so we can
// run on any Linux host (no rig needed).

#include "src/process/child_process.hpp"

#include <gtest/gtest.h>

#include <cstring>
#include <cstdio>
#include <unistd.h>

TEST(ChildProcess, StdoutPipeReadsChildOutput) {
    auto r =
        child_process::spawn("printf", {"printf", "hello"}, child_process::Direction::StdoutPipe);
    EXPECT_TRUE(r.pid > 0);
    EXPECT_TRUE(r.pipe_fd >= 0);

    char buf[16] = {};
    ssize_t n = ::read(r.pipe_fd, buf, sizeof(buf) - 1);
    EXPECT_EQ(n, 5);
    EXPECT_EQ(std::string(buf), std::string("hello"));

    child_process::reap(r.pid, r.pipe_fd, 1000);
}

TEST(ChildProcess, StdinPipeWritesToChild) {
    // `wc -c` will report the byte count of what we send on its stdin.
    // We don't read it back here; we just confirm spawning and writing
    // succeed without error. (Full read-back would require a second pipe.)
    auto r = child_process::spawn("wc", {"wc", "-c"}, child_process::Direction::StdinPipe);
    EXPECT_TRUE(r.pid > 0);
    EXPECT_TRUE(r.pipe_fd >= 0);

    const char* msg = "abcdefg";
    ssize_t n = ::write(r.pipe_fd, msg, std::strlen(msg));
    EXPECT_EQ(n, 7);
    child_process::reap(r.pid, r.pipe_fd, 1000);
}

TEST(ChildProcess, ReapHandlesAlreadyDead) {
    auto r = child_process::spawn("true", {"true"}, child_process::Direction::StdoutPipe);
    EXPECT_TRUE(r.pid > 0);
    ::usleep(50 * 1000);
    // reap() should handle a child that's already in EXIT state without
    // blocking on signals.
    child_process::reap(r.pid, r.pipe_fd, 1000);
    child_process::reap(-1, -1, 100);
}

TEST(ChildProcess, SpawnUnknownProgramFailsCleanly) {
    auto r = child_process::spawn("this_binary_does_not_exist_xyzzy",
                                  {"this_binary_does_not_exist_xyzzy"},
                                  child_process::Direction::StdoutPipe);
    // posix_spawnp may return 0 and let the child exec-fail, or it may
    // synchronously error. Either way we must NOT leak the parent's pipe fd.
    if (r.pid > 0) {
        child_process::reap(r.pid, r.pipe_fd, 1000);
    } else {
        EXPECT_TRUE(r.pipe_fd < 0);
    }
}
