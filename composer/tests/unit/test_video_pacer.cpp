#include "src/sink/video_pacer.hpp"

#include <gtest/gtest.h>

#include <vector>

namespace {
constexpr int64_t kMs = 1'000'000;
constexpr int64_t kT0 = 1'000'000'000;
constexpr int64_t kPeriod = 1'000'000'000 / 30;
} // namespace

TEST(VideoPacer, SteadyArrivalsGetConsecutiveSlots) {
    sink::VideoPacer p(30, kT0);
    for (int64_t i = 0; i < 90; ++i) {
        const auto slot = p.on_frame(kT0 + i * kPeriod + 3 * kMs);
        ASSERT_TRUE(slot.has_value());
        EXPECT_EQ(*slot, i);
    }
    EXPECT_EQ(p.repeats(), 0);
    EXPECT_EQ(p.drops(), 0);
}

TEST(VideoPacer, JitteredArrivalsStayConsecutive) {
    sink::VideoPacer p(30, kT0);
    const int64_t jitter[] = {0, 14, -12, 9, -15, 3, 16, -8};
    for (int64_t i = 0; i < 64; ++i) {
        const auto slot = p.on_frame(kT0 + i * kPeriod + jitter[i % 8] * kMs);
        ASSERT_TRUE(slot.has_value()) << i;
        EXPECT_EQ(*slot, i);
    }
    EXPECT_EQ(p.drops(), 0);
}

TEST(VideoPacer, RepeatsOncePerPeriodWhileUpstreamSilent) {
    sink::VideoPacer p(30, kT0);
    ASSERT_EQ(p.on_frame(kT0).value_or(-1), 0);
    EXPECT_FALSE(p.repeat_due(kT0 + kPeriod + kPeriod / 2).has_value());
    int64_t emitted = 0;
    for (int64_t t = kT0; t < kT0 + 1'000'000'000; t += kMs) {
        if (p.repeat_due(t))
            ++emitted;
    }
    EXPECT_GE(emitted, 27);
    EXPECT_LE(emitted, 30);
}

TEST(VideoPacer, RepeatAfterLongStallDoesNotBurst) {
    sink::VideoPacer p(30, kT0);
    ASSERT_TRUE(p.on_frame(kT0).has_value());
    const int64_t late = kT0 + 5'000'000'000;
    ASSERT_TRUE(p.repeat_due(late).has_value());
    EXPECT_FALSE(p.repeat_due(late).has_value());
    EXPECT_GE(p.last_slot(), 148);
}

TEST(VideoPacer, FramesFasterThanFpsAreDropped) {
    sink::VideoPacer p(30, kT0);
    int64_t emitted = 0;
    for (int64_t i = 0; i < 120; ++i) {
        if (p.on_frame(kT0 + i * (kPeriod / 2)))
            ++emitted;
    }
    EXPECT_LE(emitted, 62);
    EXPECT_GT(p.drops(), 50);
}

TEST(VideoPacer, NextRepeatDeadlineIsTwoSlotsAfterLast) {
    sink::VideoPacer p(30, kT0);
    ASSERT_TRUE(p.on_frame(kT0).has_value());
    EXPECT_EQ(p.next_repeat_ns(), kT0 + 2 * 1'000'000'000 / 30);
}

TEST(VideoPacer, SlowUpstreamIsFilledWithRepeats) {
    sink::VideoPacer p(30, kT0);
    std::vector<int64_t> slots;
    for (int64_t t = kT0; t < kT0 + 2'000'000'000; t += 5 * kMs) {
        if ((t - kT0) % (100 * kMs) == 0) {
            if (auto s = p.on_frame(t))
                slots.push_back(*s);
        }
        if (auto s = p.repeat_due(t))
            slots.push_back(*s);
    }
    ASSERT_GE(slots.size(), 2U);
    for (size_t i = 1; i < slots.size(); ++i)
        EXPECT_LE(slots[i] - slots[i - 1], 2) << "at " << i;
    EXPECT_GE(slots.size(), 38U);
}
