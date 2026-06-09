// Tests for classify_capture_poll — the pure poll(2) revents → action
// decision for the source capture fd. No V4L2/MPP; runs on host.
//
// Regression intent: a stalled USB capture fd raises POLLERR/POLLHUP, which
// the loop must treat as an actionable error (recover) rather than ignore.
// Ignoring it makes poll() return immediately forever => 100% CPU busy-spin.

#include "src/source/capture_poll.hpp"

#include <gtest/gtest.h>

#include <poll.h>

using source::classify_capture_poll;

TEST(CapturePoll, PollinRequestsDequeue) {
    auto a = classify_capture_poll(POLLIN);
    EXPECT_TRUE(a.dequeue);
    EXPECT_FALSE(a.drain_events);
    EXPECT_FALSE(a.error);
}

// The bug: a stalled capture fd raises POLLERR. It must be flagged as an
// error so the loop recovers, not silently ignored (which busy-spins).
TEST(CapturePoll, PollerrIsError) {
    auto a = classify_capture_poll(POLLERR);
    EXPECT_TRUE(a.error);
    EXPECT_FALSE(a.dequeue);
    EXPECT_FALSE(a.drain_events);
}

TEST(CapturePoll, PollpriRequestsDrainEvents) {
    auto a = classify_capture_poll(POLLPRI);
    EXPECT_TRUE(a.drain_events);
    EXPECT_FALSE(a.dequeue);
    EXPECT_FALSE(a.error);
}

TEST(CapturePoll, PollhupIsError) {
    EXPECT_TRUE(classify_capture_poll(POLLHUP).error);
}

TEST(CapturePoll, PollnvalIsError) {
    EXPECT_TRUE(classify_capture_poll(POLLNVAL).error);
}

TEST(CapturePoll, EventsAndFrameTogether) {
    auto a = classify_capture_poll(POLLIN | POLLPRI);
    EXPECT_TRUE(a.dequeue);
    EXPECT_TRUE(a.drain_events);
    EXPECT_FALSE(a.error);
}

// A timeout / spurious wakeup (no revents) is the no-op case — nothing to do.
TEST(CapturePoll, NoReventsIsIdle) {
    auto a = classify_capture_poll(0);
    EXPECT_FALSE(a.dequeue);
    EXPECT_FALSE(a.drain_events);
    EXPECT_FALSE(a.error);
}

// Error must still be reported even if POLLIN is also set, so the loop never
// busy-spins dequeuing a dead fd.
TEST(CapturePoll, ErrorReportedAlongsidePollin) {
    EXPECT_TRUE(classify_capture_poll(POLLIN | POLLERR).error);
}
