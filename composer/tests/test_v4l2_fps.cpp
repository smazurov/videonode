// Tests for v4l2_fps — timeperframe->fps arithmetic and the VIDIOC_G_PARM
// query path (via an injected ioctl fake; no real device).

#include "src/capture/v4l2_fps.hpp"

#include <gtest/gtest.h>

#include <linux/videodev2.h>
#include <sys/ioctl.h>

using v4l2::fps_from_timeperframe;
using v4l2::query_capture_fps;

TEST(V4l2Fps, WholeRates) {
    EXPECT_EQ(30u, fps_from_timeperframe(1, 30)); // 1/30 s per frame -> 30 fps
    EXPECT_EQ(60u, fps_from_timeperframe(1, 60));
    EXPECT_EQ(1u, fps_from_timeperframe(1, 1));
}

TEST(V4l2Fps, NtscFractionsRoundToNearest) {
    EXPECT_EQ(60u, fps_from_timeperframe(1001, 60000)); // 59.94 -> 60
    EXPECT_EQ(30u, fps_from_timeperframe(1001, 30000)); // 29.97 -> 30
    EXPECT_EQ(24u, fps_from_timeperframe(1001, 24000)); // 23.976 -> 24
}

TEST(V4l2Fps, ZeroIsSafe) {
    EXPECT_EQ(0u, fps_from_timeperframe(0, 30)); // numerator 0 -> no divide
    EXPECT_EQ(0u, fps_from_timeperframe(1, 0));  // denominator 0 -> 0 fps
}

TEST(V4l2Fps, QueryReadsTimeperframe) {
    // Fake ioctl: report 1/60 timeperframe on G_PARM, mirroring a device
    // pinned to 60 fps.
    auto io = [](int /*fd*/, unsigned long request, void* arg) -> int {
        EXPECT_EQ(static_cast<unsigned long>(VIDIOC_G_PARM), request);
        auto* parm = static_cast<v4l2_streamparm*>(arg);
        parm->parm.capture.timeperframe.numerator = 1;
        parm->parm.capture.timeperframe.denominator = 60;
        return 0;
    };
    EXPECT_EQ(60u, query_capture_fps(io, 42, V4L2_BUF_TYPE_VIDEO_CAPTURE));
}

TEST(V4l2Fps, QueryReturnsZeroOnIoctlFailure) {
    auto io = [](int /*fd*/, unsigned long /*request*/, void* /*arg*/) -> int { return -1; };
    EXPECT_EQ(0u, query_capture_fps(io, 42, V4L2_BUF_TYPE_VIDEO_CAPTURE));
}

TEST(V4l2Fps, QueryReturnsZeroOnNegativeFd) {
    bool called = false;
    auto io = [&](int /*fd*/, unsigned long /*request*/, void* /*arg*/) -> int {
        called = true;
        return 0;
    };
    EXPECT_EQ(0u, query_capture_fps(io, -1, V4L2_BUF_TYPE_VIDEO_CAPTURE));
    EXPECT_FALSE(called); // must not ioctl on a closed device
}
