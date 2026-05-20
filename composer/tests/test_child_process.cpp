// Tests for child_process — uses small `cat`/`echo` subprocesses so we can
// run on any Linux host (no rig needed).

#include "../src/child_process.hpp"
#include "test_runner.hpp"

#include <cstring>
#include <cstdio>
#include <unistd.h>

int main() {
    test_runner::start_case("stdout_pipe_reads_child_output");
    {
        // `printf 'hello'` -> we read from its stdout.
        auto r = child_process::spawn("printf", {"printf", "hello"},
                                      child_process::Direction::StdoutPipe);
        CHECK_TRUE(r.pid > 0);
        CHECK_TRUE(r.pipe_fd >= 0);

        char buf[16] = {};
        ssize_t n = ::read(r.pipe_fd, buf, sizeof(buf) - 1);
        CHECK_TRUE(n == 5);
        CHECK_STR_EQ(buf, "hello");

        child_process::reap(r.pid, r.pipe_fd, 1000);
    }

    test_runner::start_case("stdin_pipe_writes_to_child");
    {
        // `wc -c` will report the byte count of what we send on its stdin.
        // We don't read it back here; we just confirm spawning and writing
        // succeed without error. (Full read-back would require a second pipe.)
        auto r = child_process::spawn("wc", {"wc", "-c"}, child_process::Direction::StdinPipe);
        CHECK_TRUE(r.pid > 0);
        CHECK_TRUE(r.pipe_fd >= 0);

        const char* msg = "abcdefg";
        ssize_t n = ::write(r.pipe_fd, msg, std::strlen(msg));
        CHECK_TRUE(n == 7);
        child_process::reap(r.pid, r.pipe_fd, 1000);
    }

    test_runner::start_case("reap_handles_already_dead");
    {
        auto r = child_process::spawn("true", {"true"}, child_process::Direction::StdoutPipe);
        CHECK_TRUE(r.pid > 0);
        // Give it a moment to exit on its own.
        ::usleep(50 * 1000);
        // reap() should handle a child that's already in EXIT state without
        // blocking on signals.
        child_process::reap(r.pid, r.pipe_fd, 1000);
        // Re-reap is a no-op; -1 pid path.
        child_process::reap(-1, -1, 100);
    }

    test_runner::start_case("spawn_unknown_program_fails_cleanly");
    {
        auto r = child_process::spawn("this_binary_does_not_exist_xyzzy",
                                      {"this_binary_does_not_exist_xyzzy"},
                                      child_process::Direction::StdoutPipe);
        // posix_spawnp may return 0 and let the child exec-fail, or it may
        // synchronously error. Either way we must NOT leak the parent's pipe fd.
        if (r.pid > 0) {
            // Child will exec-fail and exit non-zero; reap waits for that.
            child_process::reap(r.pid, r.pipe_fd, 1000);
        } else {
            CHECK_TRUE(r.pipe_fd < 0); // helper closed it on failure
        }
    }

    return test_runner::report_and_exit_code();
}
