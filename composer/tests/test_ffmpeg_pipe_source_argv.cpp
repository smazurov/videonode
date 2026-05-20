// Tests for FfmpegPipeSource argv. Since build_argv_ is private, we test
// behaviour via the InitParams surface and verify spawn order via comments
// matching what we know to be correct. The encoder argv test is the load-
// bearing one; this one is a sanity check.

#include "../src/dma_heap.hpp"
#include "../src/ffmpeg_pipe_source.hpp"
#include "test_runner.hpp"

#include <cstdio>

int main() {
    using ffmpeg_pipe_source::FfmpegPipeSource;
    using ffmpeg_pipe_source::InitParams;

    test_runner::start_case("init_validates_dims");
    {
        FfmpegPipeSource s;
        InitParams p{};
        p.device = "/dev/null";
        p.input_format = "nv12";
        p.width = 1921; // odd -> should be rejected
        p.height = 1080;
        CHECK_TRUE(!s.init(p));
    }

    test_runner::start_case("init_succeeds_for_valid");
    {
        // init() touches /dev/dma_heap/system to allocate the ring of
        // capture buffers. On generic Linux dev machines that node exists
        // but is owned by `video` group and unreadable to ordinary users;
        // on the rig the `orangepi` user is in `video` so this works.
        // Probe first; skip the body when allocation isn't available so
        // the rest of the suite still reports clean.
        auto probe = dmaheap::alloc("system", 4096);
        if (!probe.valid()) {
            fprintf(stderr, "skip: /dev/dma_heap/system not accessible "
                            "as this user (need group `video` membership)\n");
        } else {
            FfmpegPipeSource s;
            InitParams p{};
            p.device = "/dev/null";
            p.input_format = "nv12";
            p.width = 1920;
            p.height = 1080;
            CHECK_TRUE(s.init(p));
            CHECK_EQ(s.width(), 1920);
            CHECK_EQ(s.height(), 1080);
        }
    }

    return test_runner::report_and_exit_code();
}
