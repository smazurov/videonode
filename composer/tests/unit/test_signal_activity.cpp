#include "src/capture/signal_activity.hpp"

#include <gtest/gtest.h>

#include <cstdint>
#include <vector>

using signal_activity::compute_luma_stats;
using signal_activity::Detector;
using signal_activity::LumaStats;
using signal_activity::LumaView;

namespace {

constexpr int kW = 128;
constexpr int kH = 72;

std::vector<uint8_t> nv12_y(uint8_t fill) {
    return std::vector<uint8_t>(size_t(kW) * kH, fill);
}

LumaView nv12_view(const std::vector<uint8_t>& buf) {
    return {.data = buf, .width = kW, .height = kH, .row_pitch = kW, .pixel_stride = 1};
}

} // namespace

TEST(SignalActivity, FlatFrameHasZeroVariance) {
    auto buf = nv12_y(16);
    LumaStats s = compute_luma_stats(nv12_view(buf));
    ASSERT_TRUE(s.valid);
    EXPECT_NEAR(s.variance, 0.0, 0.001);
}

TEST(SignalActivity, GradientHasVariance) {
    std::vector<uint8_t> buf(size_t(kW) * kH);
    for (int y = 0; y < kH; ++y)
        for (int x = 0; x < kW; ++x)
            buf[size_t(y) * kW + x] = static_cast<uint8_t>(x * 2);
    LumaStats s = compute_luma_stats(nv12_view(buf));
    ASSERT_TRUE(s.valid);
    EXPECT_GT(s.variance, 100.0);
}

TEST(SignalActivity, FlatFrozenStreamGoesDeadAfterDeadline) {
    Detector d;
    d.set_thresholds(/*flat_variance=*/6.0, /*dead_frames=*/5);
    auto black = nv12_y(16);
    LumaStats s = compute_luma_stats(nv12_view(black));
    // First frame has no predecessor to compare, so the streak needs
    // dead_frames + 1 updates to trip.
    for (int i = 0; i < 5; ++i) {
        EXPECT_FALSE(d.update(s)) << "update " << i;
    }
    EXPECT_TRUE(d.update(s));
}

TEST(SignalActivity, ChangingContentNeverDead) {
    Detector d;
    d.set_thresholds(6.0, 3);
    for (int i = 0; i < 20; ++i) {
        std::vector<uint8_t> buf(size_t(kW) * kH);
        for (size_t k = 0; k < buf.size(); ++k)
            buf[k] = static_cast<uint8_t>((k + i) * 7);
        EXPECT_FALSE(d.update(compute_luma_stats(nv12_view(buf))));
    }
}

TEST(SignalActivity, FlatButChangingLevelNotDead) {
    Detector d;
    d.set_thresholds(6.0, 3);
    for (int i = 0; i < 20; ++i) {
        auto buf = nv12_y(static_cast<uint8_t>(16 + (i % 3) * 20));
        EXPECT_FALSE(d.update(compute_luma_stats(nv12_view(buf))));
    }
}

TEST(SignalActivity, RecoversOnFirstLiveFrame) {
    Detector d;
    d.set_thresholds(6.0, 3);
    auto black = nv12_y(16);
    LumaStats dead_stats = compute_luma_stats(nv12_view(black));
    for (int i = 0; i < 10; ++i)
        (void)d.update(dead_stats);
    EXPECT_TRUE(d.update(dead_stats));

    std::vector<uint8_t> live(size_t(kW) * kH);
    for (size_t k = 0; k < live.size(); ++k)
        live[k] = static_cast<uint8_t>(k * 13);
    EXPECT_FALSE(d.update(compute_luma_stats(nv12_view(live))));
}

TEST(SignalActivity, PackedYuyvLumaSampled) {
    // YUYV: Y at even bytes; fill luma flat, chroma noisy -> still reads flat.
    std::vector<uint8_t> buf(size_t(kW) * kH * 2);
    for (size_t p = 0; p < buf.size(); p += 2) {
        buf[p] = 16;                                   // Y
        buf[p + 1] = static_cast<uint8_t>(p * 31 + 7); // chroma noise
    }
    LumaView v{.data = buf,
               .width = kW,
               .height = kH,
               .row_pitch = kW * 2,
               .pixel_stride = 2,
               .sample_offset = 0};
    LumaStats s = compute_luma_stats(v);
    ASSERT_TRUE(s.valid);
    EXPECT_NEAR(s.variance, 0.0, 0.001);
}
