#include "src/source/out_ring_gate.hpp"

#include <gtest/gtest.h>

#include <cstdint>
#include <set>

TEST(OutRingGate, PicksStartWhenFree) {
    auto s = source::reserve_out_slot(4, 2, [](uint32_t) { return true; });
    ASSERT_TRUE(s.has_value());
    EXPECT_EQ(uint32_t(2), *s);
}

TEST(OutRingGate, SkipsBusyScanningForward) {
    std::set<uint32_t> busy = {2, 3};
    auto s = source::reserve_out_slot(4, 2, [&](uint32_t i) { return busy.count(i) == 0; });
    ASSERT_TRUE(s.has_value());
    EXPECT_EQ(uint32_t(0), *s);
}

TEST(OutRingGate, WrapsAround) {
    std::set<uint32_t> busy = {3, 0, 1};
    auto s = source::reserve_out_slot(4, 3, [&](uint32_t i) { return busy.count(i) == 0; });
    ASSERT_TRUE(s.has_value());
    EXPECT_EQ(uint32_t(2), *s);
}

TEST(OutRingGate, NoneFreeReturnsNullopt) {
    auto s = source::reserve_out_slot(4, 0, [](uint32_t) { return false; });
    EXPECT_FALSE(s.has_value());
}

TEST(OutRingGate, ZeroRingReturnsNullopt) {
    auto s = source::reserve_out_slot(0, 0, [](uint32_t) { return true; });
    EXPECT_FALSE(s.has_value());
}
