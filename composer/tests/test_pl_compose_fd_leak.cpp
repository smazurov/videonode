// Regression: PlCompose::render() dups dma-buf fds for libplacebo
// texture import but never closes them. At 60fps the composer exhausts
// RLIMIT_NOFILE in ~8 seconds, blocking SCM_RIGHTS frame delivery.

#include "src/render/gbm_alloc.hpp"
#include "src/render/pl_compose.hpp"

#include <gtest/gtest.h>

#include <dirent.h>
#include <unistd.h>

namespace {

int count_open_fds() {
    DIR* d = opendir("/proc/self/fd");
    if (!d)
        return -1;
    int n = 0;
    while (readdir(d) != nullptr)
        ++n;
    closedir(d);
    return n - 2; // subtract "." and ".."
}

constexpr int kW = 64;
constexpr int kH = 64;
constexpr int kFrames = 100;
constexpr int kMaxFdGrowth = 4;

} // namespace

TEST(PlComposeFdLeak, RenderDoesNotLeakDmabufFds) {
    pl_compose::PlCompose comp;
    if (!comp.init("/dev/dri/renderD128", kW, kH)) {
        GTEST_SKIP() << "PlCompose::init failed — no DRM render node";
    }

    gbm_device* gbm = comp.gbm();
    ASSERT_NE(gbm, nullptr);
    gbm_alloc::Nv12Buf src = gbm_alloc::alloc(gbm, kW, kH);
    ASSERT_TRUE(src.valid()) << "GBM NV12 alloc failed";

    pl_compose::SourceSlot slot;
    slot.src_y_fd = src.y_fd;
    slot.src_uv_fd = src.uv_fd;
    slot.src_w = kW;
    slot.src_h = kH;
    slot.src_y_pitch = static_cast<int>(src.y_stride);
    slot.src_uv_pitch = static_cast<int>(src.uv_stride);
    slot.x = 0;
    slot.y = 0;
    slot.w = kW;
    slot.h = kH;

    int fd_before = count_open_fds();
    ASSERT_GT(fd_before, 0);

    for (int i = 0; i < kFrames; ++i) {
        ASSERT_TRUE(comp.render({slot}));
        comp.finish();
    }

    int fd_after = count_open_fds();
    int leaked = fd_after - fd_before;
    EXPECT_LE(leaked, kMaxFdGrowth) << "fd count grew by " << leaked << " over " << kFrames
                                    << " frames (expected <= " << kMaxFdGrowth << ")";

    gbm_alloc::free(src);
}
