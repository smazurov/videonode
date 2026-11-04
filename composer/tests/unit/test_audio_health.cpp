#include "src/sink/audio_health.hpp"

#include <gtest/gtest.h>

#include <string>
#include <vector>

namespace {

constexpr int64_t kSec = 1'000'000'000;

struct Lines {
    std::vector<std::pair<bool, std::string>> got;
    sink::AudioHealthLog::LogFn fn() {
        return [this](bool warn, const std::string& line) { got.emplace_back(warn, line); };
    }
};

} // namespace

TEST(AudioHealthLog, LogsEachTransitionOnce) {
    Lines l;
    sink::AudioHealthLog h("[0] dev", l.fn());
    h.on_state(sink::DeviceState::Live, "", 0);
    h.on_state(sink::DeviceState::Live, "", 1);
    h.on_state(sink::DeviceState::Absent, "No such device", 10 * kSec);
    h.on_state(sink::DeviceState::Absent, "No such device", 11 * kSec);
    h.on_state(sink::DeviceState::Live, "", 22 * kSec);
    ASSERT_EQ(l.got.size(), 3U);
    EXPECT_FALSE(l.got[0].first);
    EXPECT_TRUE(l.got[1].first);
    EXPECT_NE(l.got[1].second.find("missing; sending silence, retrying (No such device)"),
              std::string::npos);
    EXPECT_NE(l.got[2].second.find("live again after 12.0s missing"), std::string::npos);
}

TEST(AudioHealthLog, OpeningRetriesDoNotRelog) {
    Lines l;
    sink::AudioHealthLog h("[0] dev", l.fn());
    h.on_state(sink::DeviceState::Absent, "x", 0);
    for (int k = 1; k < 100; ++k) {
        h.on_state(sink::DeviceState::Opening, "", k * kSec / 2);
        h.on_state(sink::DeviceState::Absent, "x", k * kSec / 2);
    }
    EXPECT_EQ(l.got.size(), 1U);
}

TEST(AudioHealthLog, FlappingIsFoldedIntoSummary) {
    Lines l;
    sink::AudioHealthLog h("[0] dev", l.fn());
    h.tick(0, 0, 0);
    h.on_state(sink::DeviceState::Live, "", 0);
    for (int k = 1; k <= 20; ++k)
        h.on_state(k % 2 != 0 ? sink::DeviceState::Stalled : sink::DeviceState::Live, "",
                   k * kSec / 10);
    EXPECT_EQ(l.got.size(), 1U);
    h.tick(31 * kSec, 0, 0);
    ASSERT_EQ(l.got.size(), 2U);
    EXPECT_NE(l.got.back().second.find("20 state changes not logged; now live"), std::string::npos);
}

TEST(AudioHealthLog, FlapEndingDegradedIsLoggedOnceSettled) {
    Lines l;
    sink::AudioHealthLog h("[0] dev", l.fn());
    h.tick(0, 0, 0);
    h.on_state(sink::DeviceState::Live, "", 0);
    h.on_state(sink::DeviceState::Stalled, "", kSec);
    h.tick(2 * kSec, 0, 0);
    EXPECT_EQ(l.got.size(), 1U);
    h.tick(6 * kSec, 0, 0);
    ASSERT_EQ(l.got.size(), 2U);
    EXPECT_TRUE(l.got.back().first);
    EXPECT_NE(l.got.back().second.find("no samples; sending silence"), std::string::npos);
    h.tick(7 * kSec, 0, 0);
    EXPECT_EQ(l.got.size(), 2U);
}

TEST(AudioHealthLog, SummaryOnlyWhenSomethingHappened) {
    Lines l;
    sink::AudioHealthLog h("[0] dev", l.fn());
    h.tick(0, 0, 0);
    h.tick(31 * kSec, 0, 0);
    EXPECT_TRUE(l.got.empty());
    h.on_xrun();
    h.on_xrun();
    h.tick(40 * kSec, 24000, 0);
    EXPECT_TRUE(l.got.empty());
    h.tick(62 * kSec, 24000, 4800);
    ASSERT_EQ(l.got.size(), 1U);
    EXPECT_NE(l.got[0].second.find("2 xruns"), std::string::npos);
    EXPECT_NE(l.got[0].second.find("0.5s silence filled"), std::string::npos);
    EXPECT_NE(l.got[0].second.find("0.1s late audio dropped"), std::string::npos);
    h.tick(93 * kSec, 24000, 4800);
    EXPECT_EQ(l.got.size(), 1U);
}
