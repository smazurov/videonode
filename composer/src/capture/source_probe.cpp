#include "src/capture/source_probe.hpp"

#include "src/capture/v4l2_capture.hpp"
#include "src/common/log_levels.hpp"

#include <cerrno>
#include <cstring>
#include <linux/videodev2.h>

namespace source_probe {

namespace {
// Threshold past which Live degrades to Transitioning (DQBUF failed N
// times in a row). Picked to absorb single missed frames but flag real
// stalls. We're not doing time-based decisions — this is event count.
constexpr int kFailuresBeforeTransitioning = 3;
} // namespace

const char* status_text(Health h) {
    switch (h) {
    case Health::Probing:
        return "INITIALIZING";
    case Health::Live:
        return "LIVE";
    case Health::Transitioning:
        return "SIGNAL CHANGING";
    case Health::NoCable:
        return "NO CABLE DETECTED";
    case Health::NoLock:
        return "WAITING FOR SIGNAL";
    case Health::Gone:
        return "DEVICE OFFLINE";
    }
    return "UNKNOWN";
}

const char* health_token(Health h) {
    switch (h) {
    case Health::Probing:
        return "initializing";
    case Health::Live:
        return "live";
    case Health::Transitioning:
        return "transitioning";
    case Health::NoCable:
        return "no_cable";
    case Health::NoLock:
        return "no_signal";
    case Health::Gone:
        return "offline";
    }
    return "unknown";
}

SourceProbe::SourceProbe(v4l2::Streamer& cap) : cap_(cap) {}

bool SourceProbe::attach() {
    bool subscribed = cap_.subscribe_source_change();
    int subscribe_errno = errno;
    // Cable / signal truth comes from VIDIOC_QUERY_DV_TIMINGS, not from
    // V4L2_CID_DV_RX_POWER_PRESENT (rk_hdmirx reports power_present=1 even
    // with the cable physically unplugged). The Go side learned this the
    // hard way — see pkg/linuxav/v4l2/signal.go.
    auto s = cap_.query_dv_timings_state();
    if (s != v4l2::Streamer::DvTimingsState::NotSupported &&
        s != v4l2::Streamer::DvTimingsState::OtherError) {
        has_dv_timings_ = true;
        apply_dv_timings_state(s);
        vn::log::info("source_probe: HDMI mode, dv_timings=%s", dv_timings_label(s));
        // SOURCE_CHANGE failure only matters on HDMI-class devices; UVC
        // webcams reject the subscribe and that's expected.
        if (!subscribed)
            vn::log::error("v4l2_capture: VIDIOC_SUBSCRIBE_EVENT: %s", strerror(subscribe_errno));
    }
    return true;
}

void SourceProbe::note_event(const v4l2_event& e) {
    if (e.type == V4L2_EVENT_SOURCE_CHANGE) {
        source_change_pending_ = true;
        // SOURCE_CHANGE doesn't tell us cable-state directly; the timings
        // probe does. Re-query immediately so a cable yank that came in
        // along with the SOURCE_CHANGE flips state without waiting for
        // the next 1 Hz backstop tick.
        refresh_dv_timings();
        return;
    }
}

void SourceProbe::refresh_power_present() {
    refresh_dv_timings();
}

void SourceProbe::refresh_dv_timings() {
    if (!has_dv_timings_)
        return;
    apply_dv_timings_state(cap_.query_dv_timings_state());
}

const char* SourceProbe::dv_timings_label(v4l2::Streamer::DvTimingsState s) {
    using S = v4l2::Streamer::DvTimingsState;
    switch (s) {
    case S::Locked:
        return "locked";
    case S::NoLink:
        return "no-link";
    case S::Unstable:
        return "unstable";
    case S::OutOfRange:
        return "out-of-range";
    case S::NotSupported:
        return "not-supported";
    case S::OtherError:
        return "error";
    }
    return "?";
}

void SourceProbe::apply_dv_timings_state(v4l2::Streamer::DvTimingsState s) {
    using S = v4l2::Streamer::DvTimingsState;
    bool new_cable = (s != S::NoLink);
    bool new_lock = (s == S::Locked);
    if (s == dv_timings_state_)
        return;
    dv_timings_state_ = s;
    cable_present_ = new_cable;
    signal_locked_ = new_lock;
    if (!new_cable) {
        // cable yanked — invalidate previous "ever live" so we report
        // NoCable cleanly on next plug, not a stale Live.
        ever_live_ = false;
        source_change_pending_ = false;
        consecutive_failures_ = 0;
    }
    vn::log::info("source_probe: dv_timings -> %s", dv_timings_label(s));
}

void SourceProbe::note_dqbuf_success() {
    ever_live_ = true;
    source_change_pending_ = false;
    consecutive_failures_ = 0;
}

void SourceProbe::note_dqbuf_failure(int e) {
    if (e == ENODEV || e == EBADF) {
        device_gone_ = true;
        return;
    }
    if (e == ETIMEDOUT || e == EAGAIN || e == EIO) {
        ++consecutive_failures_;
    }
}

void SourceProbe::note_streaming_restarted() {
    // After STREAMOFF/QBUF*/STREAMON the source-change is acknowledged.
    source_change_pending_ = false;
    consecutive_failures_ = 0;
}

void SourceProbe::note_format_change() {
    // Treat a control-plane set_format the same as a driver-originated
    // SOURCE_CHANGE: force Transitioning until the first DQBUF on the
    // reopened device succeeds.
    source_change_pending_ = true;
}

const char* SourceProbe::dv_timings_label_public(v4l2::Streamer::DvTimingsState s) {
    return dv_timings_label(s);
}

Health SourceProbe::health() const {
    if (device_gone_)
        return Health::Gone;
    // DV-timings probe is the truth for HDMI: ENOLINK → cable out.
    if (has_dv_timings_ && !cable_present_)
        return Health::NoCable;
    if (source_change_pending_)
        return Health::Transitioning;
    if (ever_live_) {
        if (consecutive_failures_ >= kFailuresBeforeTransitioning) {
            return has_dv_timings_ ? Health::Transitioning : Health::NoLock;
        }
        return Health::Live;
    }
    // Cable in, no DQBUF yet → either still locking (Unstable) or briefly
    // mid-handshake (Locked but capture not flowing yet).
    if (has_dv_timings_ && cable_present_)
        return Health::NoLock;
    return Health::Probing;
}

} // namespace source_probe
