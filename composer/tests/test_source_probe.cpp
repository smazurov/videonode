// Unit tests for source_probe. State transitions only — no real V4L2.
// Streamer is held by reference but its methods are not called from
// note_event / note_dqbuf_* paths; only attach() touches Streamer, and
// we don't test attach() here (needs a real device).

#include "../src/source_probe.hpp"
#include "../src/v4l2_capture.hpp"
#include "test_runner.hpp"

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

static void test_initial_state_probing() {
    v4l2::Streamer cap;
    SourceProbe p(cap);
    CHECK_EQ(int(Health::Probing), int(p.health()));
}

static void test_first_dqbuf_becomes_live() {
    v4l2::Streamer cap;
    SourceProbe p(cap);
    p.note_dqbuf_success();
    CHECK_EQ(int(Health::Live), int(p.health()));
}

static void test_repeated_failures_after_live_go_to_nolock() {
    v4l2::Streamer cap;
    SourceProbe p(cap);
    p.note_dqbuf_success();
    p.note_dqbuf_failure(ETIMEDOUT);
    p.note_dqbuf_failure(ETIMEDOUT);
    p.note_dqbuf_failure(ETIMEDOUT);
    // Three failures (>= threshold). HDMI mode would say Transitioning;
    // non-HDMI says NoLock. Probe in this test is non-HDMI.
    CHECK_EQ(int(Health::NoLock), int(p.health()));
}

static void test_source_change_event_marks_transitioning() {
    v4l2::Streamer cap;
    SourceProbe p(cap);
    p.note_dqbuf_success();
    p.note_event(make_source_change_event());
    CHECK_EQ(int(Health::Transitioning), int(p.health()));
}

static void test_streaming_restarted_clears_transitioning() {
    v4l2::Streamer cap;
    SourceProbe p(cap);
    p.note_dqbuf_success();
    p.note_event(make_source_change_event());
    p.note_streaming_restarted();
    CHECK_EQ(int(Health::Live), int(p.health()));
}

static void test_enodev_goes_gone() {
    v4l2::Streamer cap;
    SourceProbe p(cap);
    p.note_dqbuf_success();
    p.note_dqbuf_failure(ENODEV);
    CHECK_EQ(int(Health::Gone), int(p.health()));
}

static void test_status_text_covers_all_states() {
    CHECK_TRUE(source_probe::status_text(Health::Probing));
    CHECK_TRUE(source_probe::status_text(Health::Live));
    CHECK_TRUE(source_probe::status_text(Health::Transitioning));
    CHECK_TRUE(source_probe::status_text(Health::NoCable));
    CHECK_TRUE(source_probe::status_text(Health::NoLock));
    CHECK_TRUE(source_probe::status_text(Health::Gone));
}

int main() {
    test_runner::start_case("initial_state_probing");
    test_initial_state_probing();
    test_runner::start_case("first_dqbuf_becomes_live");
    test_first_dqbuf_becomes_live();
    test_runner::start_case("repeated_failures_to_nolock");
    test_repeated_failures_after_live_go_to_nolock();
    test_runner::start_case("source_change_marks_transitioning");
    test_source_change_event_marks_transitioning();
    test_runner::start_case("streaming_restarted_clears");
    test_streaming_restarted_clears_transitioning();
    test_runner::start_case("enodev_goes_gone");
    test_enodev_goes_gone();
    test_runner::start_case("status_text_all");
    test_status_text_covers_all_states();
    return test_runner::report_and_exit_code();
}
