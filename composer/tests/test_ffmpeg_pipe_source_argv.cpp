#include "src/ipc/dma_heap.hpp"
#include "src/process/ffmpeg_pipe_source.hpp"
#include "src/render/egl_ctx.hpp"

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
    // init() needs two host resources: /dev/dma_heap/system for the
    // capture-buffer ring and a gbm_device for the dma-buf-importable
    // NV12 ring (phase1 introduced the gbm requirement so radeonsi /
    // panthor / anv accept the import). Both are gated by user
    // permissions / hardware; skip cleanly when either is missing so
    // the rest of the suite reports clean on minimal CI hosts.
    auto probe = dmaheap::alloc("system", 4096);
    if (!probe.valid()) {
        GTEST_SKIP() << "/dev/dma_heap/system not accessible as this user "
                        "(need group `video` membership)";
    }
    egl_ctx::EglCtx ctx;
    if (!ctx.init()) {
        GTEST_SKIP() << "no DRM render node with EGL/GBM (need a /dev/dri/renderD12N "
                        "device and Mesa userspace)";
    }

    FfmpegPipeSource s;
    InitParams p{};
    p.device = "/dev/null";
    p.input_format = "nv12";
    p.width = 1920;
    p.height = 1080;
    p.gbm = ctx.gbm();
    EXPECT_TRUE(s.init(p));
    EXPECT_EQ(s.width(), 1920);
    EXPECT_EQ(s.height(), 1080);
}
