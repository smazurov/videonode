// Tests for FfmpegPipeSource argv. Since build_argv_ is private, we test
// behaviour via the InitParams surface and verify spawn order via comments
// matching what we know to be correct. The encoder argv test is the load-
// bearing one; this one is a sanity check.

#include "src/ipc/dma_heap.hpp"
#include "src/process/ffmpeg_pipe_source.hpp"

#include <gtest/gtest.h>

#include <cstdio>

using ffmpeg_pipe_source::FfmpegPipeSource;
using ffmpeg_pipe_source::InitParams;

TEST(FfmpegPipeSource, InitValidatesDims) {
    FfmpegPipeSource s;
    InitParams p{};
    p.device = "/dev/null";
    p.input_format = "nv12";
    p.width = 1921; // odd -> should be rejected
    p.height = 1080;
    EXPECT_FALSE(s.init(p));
}

TEST(FfmpegPipeSource, InitSucceedsForValid) {
    // init() touches /dev/dma_heap/system to allocate the ring of
    // capture buffers. On generic Linux dev machines that node exists
    // but is owned by `video` group and unreadable to ordinary users;
    // on the rig the `orangepi` user is in `video` so this works.
    // Probe first; skip the body when allocation isn't available so
    // the rest of the suite still reports clean.
    auto probe = dmaheap::alloc("system", 4096);
    if (!probe.valid()) {
        GTEST_SKIP() << "/dev/dma_heap/system not accessible as this user "
                        "(need group `video` membership)";
    }
    FfmpegPipeSource s;
    InitParams p{};
    p.device = "/dev/null";
    p.input_format = "nv12";
    p.width = 1920;
    p.height = 1080;
    EXPECT_TRUE(s.init(p));
    EXPECT_EQ(s.width(), 1920);
    EXPECT_EQ(s.height(), 1080);
}
