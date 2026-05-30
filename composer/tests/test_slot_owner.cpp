#include "src/source/slot_owner.hpp"

#include <gtest/gtest.h>

#include <atomic>
#include <thread>
#include <vector>

namespace {

using source::SlotOwner;

TEST(SlotOwner, PicksFreeAndGatesBusy) {
    SlotOwner s(3);
    EXPECT_EQ(s.pick_free_slot(), 0);
    uint64_t g = s.begin_write(0);
    s.add_refs(0, 2);
    EXPECT_EQ(s.refcount(0), 2);
    EXPECT_EQ(s.pick_free_slot(), 1);

    s.release(0, g);
    EXPECT_EQ(s.refcount(0), 1);
    s.release(0, g);
    EXPECT_EQ(s.refcount(0), 0);
    EXPECT_EQ(s.pick_free_slot(), 0);
}

TEST(SlotOwner, StaleGenerationCreditIgnored) {
    SlotOwner s(2);
    uint64_t g0 = s.begin_write(0);
    s.add_refs(0, 1);
    s.release(0, g0);
    EXPECT_EQ(s.refcount(0), 0);

    uint64_t g1 = s.begin_write(0); // recycle
    s.add_refs(0, 1);
    s.release(0, g0); // late credit for the prior occupancy
    EXPECT_EQ(s.refcount(0), 1) << "stale-gen credit must not free a recycled slot";
    s.release(0, g1);
    EXPECT_EQ(s.refcount(0), 0);
}

TEST(SlotOwner, ReleaseNeverUnderflows) {
    SlotOwner s(1);
    uint64_t g = s.begin_write(0);
    s.release(0, g); // refcount already 0
    s.release(0, g);
    EXPECT_EQ(s.refcount(0), 0);
}

TEST(SlotOwner, PinFailsOnStaleGeneration) {
    SlotOwner s(1);
    uint64_t g0 = s.begin_write(0);
    EXPECT_TRUE(s.pin(0, g0));
    EXPECT_EQ(s.refcount(0), 1);
    s.release(0, g0);

    uint64_t g1 = s.begin_write(0);
    EXPECT_FALSE(s.pin(0, g0)) << "pin against stale gen must fail and not leak a ref";
    EXPECT_EQ(s.refcount(0), 0);
    EXPECT_TRUE(s.pin(0, g1));
    s.release(0, g1);
}

TEST(SlotOwner, FullRingReturnsNoFreeSlot) {
    SlotOwner s(2);
    s.begin_write(0);
    s.add_refs(0, 1);
    s.begin_write(1);
    s.add_refs(1, 1);
    EXPECT_EQ(s.pick_free_slot(), -1);
}

TEST(SlotOwner, ConcurrentReleaseMatchesAddedRefs) {
    SlotOwner s(1);
    uint64_t g = s.begin_write(0);
    constexpr int kReaders = 8;
    s.add_refs(0, kReaders);
    std::vector<std::thread> ts;
    ts.reserve(kReaders);
    for (int i = 0; i < kReaders; ++i)
        ts.emplace_back([&] { s.release(0, g); });
    for (auto& t : ts)
        t.join();
    EXPECT_EQ(s.refcount(0), 0);
}

} // namespace
