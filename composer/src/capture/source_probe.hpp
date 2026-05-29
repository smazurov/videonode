// source_probe — derives a source's health state from V4L2 events + DQBUF
// outcomes + VIDIOC_QUERY_DV_TIMINGS, replacing the time-threshold
// heuristic. Source-agnostic: HDMI receivers use the DV-timings probe for
// cable/signal truth (DV_RX_POWER_PRESENT is unreliable on rk_hdmirx);
// UVC-style devices fall back to DQBUF + ENODEV.

#pragma once

#include "src/capture/v4l2_capture.hpp"

#include <cstdint>

struct v4l2_event;

namespace source_probe {

// Health states. Maps 1:1 to placeholder messages.
enum class Health {
    Probing,       // sidecar started, no decision yet
    Live,          // real frames flowing
    Transitioning, // SOURCE_CHANGE arrived or short DQBUF stall; hold last frame
    NoCable,       // QUERY_DV_TIMINGS returns ENOLINK (HDMI only)
    NoLock,        // cable plugged but no usable signal
    Gone,          // device absent (USB unplug / node gone) or hard error;
                   // reported on the wire as no_signal (not offline)
};

// status_text returns a short label suitable for the placeholder painter.
const char* status_text(Health h);

// health_token returns the stable machine token reported on the wire via
// gRPC Status.health: live | transitioning | no_cable | no_signal |
// initializing. Distinct from status_text (human overlay). The source binary
// never emits "offline" — that is the Go daemon's process-down signal.
const char* health_token(Health h);

class SourceProbe {
  public:
    explicit SourceProbe(v4l2::Streamer& cap);

    // attach subscribes to V4L2_EVENT_SOURCE_CHANGE and probes the DV
    // timings ioctl once to detect whether this device is an HDMI receiver
    // (has timings) or a non-HDMI device (use DQBUF-only health).
    bool attach();

    void note_event(const v4l2_event& e);
    void note_dqbuf_success();
    void note_dqbuf_failure(int errno_val);

    // note_device_absent is called by the reopen loop when an attempt finds
    // the device gone (ENOENT/ENODEV) or hard-fails. Latches the absent state
    // (reported as no_signal) until the next successful open or DQBUF.
    void note_device_absent();

    // note_device_acquiring is called when the node is present but not yet
    // openable (EBUSY/EACCES — udev settle window). Drops the absent latch so
    // health reports the bring-up state (initializing) rather than no_signal.
    void note_device_acquiring();

    // mark_renegotiating is called by the main loop after it consumed a
    // SOURCE_CHANGE and successfully restarted streaming, so the probe
    // can transition out of Transitioning on the next successful DQBUF.
    void note_streaming_restarted();

    // note_format_change is called by the control-plane handler before
    // tearing down + reopening the capture device with new args. Puts
    // the probe into Transitioning so the main loop knows we're in flux;
    // cleared by the next note_dqbuf_success().
    void note_format_change();

    [[nodiscard]] Health health() const;

    // dv_timings_state exposes the cached DV timings probe result so the
    // control-plane status snapshot can include the label without
    // re-querying the ioctl.
    [[nodiscard]] v4l2::Streamer::DvTimingsState dv_timings_state() const {
        return dv_timings_state_;
    }

    // dv_timings_label_for is the public form of the private label
    // helper, exposed so the status builder can render the enum.
    static const char* dv_timings_label_public(v4l2::Streamer::DvTimingsState s);

    // diagnostic accessors for logging
    [[nodiscard]] bool has_dv_timings() const { return has_dv_timings_; }
    [[nodiscard]] bool cable_present() const { return cable_present_; }
    [[nodiscard]] bool signal_locked() const { return signal_locked_; }

    // refresh_dv_timings calls VIDIOC_QUERY_DV_TIMINGS right now and
    // updates internal state. Called on every SOURCE_CHANGE event and once
    // per second by the main loop as a backstop.
    void refresh_dv_timings();

    // Back-compat alias kept for any caller still using the old name.
    void refresh_power_present();

  private:
    void apply_dv_timings_state(v4l2::Streamer::DvTimingsState s);
    static const char* dv_timings_label(v4l2::Streamer::DvTimingsState s);

    v4l2::Streamer& cap_;
    bool has_dv_timings_ = false;
    bool cable_present_ = false;
    bool signal_locked_ = false;
    v4l2::Streamer::DvTimingsState dv_timings_state_ = v4l2::Streamer::DvTimingsState::NotSupported;
    bool source_change_pending_ = false;
    bool ever_live_ = false;
    bool device_gone_ = false;
    int consecutive_failures_ = 0;
};

} // namespace source_probe
