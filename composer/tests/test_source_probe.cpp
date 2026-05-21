// Unit tests for source_probe. State transitions only — no real V4L2.
// Streamer is held by reference but its methods are not called from
// note_event / note_dqbuf_* paths; only attach() touches Streamer, and
// we don't test attach() here (needs a real device).

#include "../src/source_probe.hpp"
#include "../src/v4l2_capture.hpp"

#include <gtest/gtest.h>

#include <linux/v4l2-controls.h>
#include <linux/videodev2.h>

using source_probe::Health;
using source_probe::SourceProbe;

namespace {

// Synthesize an event without going through the kernel.
v4l2_event make_source_change_event() {
    v4l2_event e{};
    e.type = V4L2_EVENT_SOURCE_CHANGE;
    e.u.src_change.changes = V4L2_EVENT_SRC_CH_RESOLUTION;
    return e;
}

// Note: the DV-timings cable/signal path is now driven by VIDIOC_QUERY_DV_TIMINGS
// inside SourceProbe::attach() / refresh_dv_timings(). Those require a real
// device, so we can't unit-test the HDMI mode here. Tests below exercise the
// non-HDMI (DQBUF-only) branch.

} // namespace

TEST(SourceProbe, InitialStateProbing) {
    v4l2::Streamer cap;
    SourceProbe p(cap);
    EXPECT_EQ(int(Health::Probing), int(p.health()));
}

TEST(SourceProbe, FirstDqbufBecomesLive) {
    v4l2::Streamer cap;
    SourceProbe p(cap);
    p.note_dqbuf_success();
    EXPECT_EQ(int(Health::Live), int(p.health()));
}

TEST(SourceProbe, RepeatedFailuresAfterLiveGoToNoLock) {
    v4l2::Streamer cap;
    SourceProbe p(cap);
    p.note_dqbuf_success();
    p.note_dqbuf_failure(ETIMEDOUT);
    p.note_dqbuf_failure(ETIMEDOUT);
    p.note_dqbuf_failure(ETIMEDOUT);
    // Three failures (>= threshold). HDMI mode would say Transitioning;
    // non-HDMI says NoLock. Probe in this test is non-HDMI.
    EXPECT_EQ(int(Health::NoLock), int(p.health()));
}

TEST(SourceProbe, SourceChangeEventMarksTransitioning) {
    v4l2::Streamer cap;
    SourceProbe p(cap);
    p.note_dqbuf_success();
    p.note_event(make_source_change_event());
    EXPECT_EQ(int(Health::Transitioning), int(p.health()));
}

TEST(SourceProbe, StreamingRestartedClearsTransitioning) {
    v4l2::Streamer cap;
    SourceProbe p(cap);
    p.note_dqbuf_success();
    p.note_event(make_source_change_event());
    p.note_streaming_restarted();
    EXPECT_EQ(int(Health::Live), int(p.health()));
}

TEST(SourceProbe, EnodevGoesGone) {
    v4l2::Streamer cap;
    SourceProbe p(cap);
    p.note_dqbuf_success();
    p.note_dqbuf_failure(ENODEV);
    EXPECT_EQ(int(Health::Gone), int(p.health()));
}

TEST(SourceProbe, StatusTextCoversAllStates) {
    EXPECT_TRUE(source_probe::status_text(Health::Probing));
    EXPECT_TRUE(source_probe::status_text(Health::Live));
    EXPECT_TRUE(source_probe::status_text(Health::Transitioning));
    EXPECT_TRUE(source_probe::status_text(Health::NoCable));
    EXPECT_TRUE(source_probe::status_text(Health::NoLock));
    EXPECT_TRUE(source_probe::status_text(Health::Gone));
}
