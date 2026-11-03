#include "src/sink/audio_health.hpp"

#include <array>
#include <cstdio>
#include <utility>

namespace sink {

namespace {

constexpr double kNsPerSec = 1e9;

std::string fmt_seconds(double s) {
    std::array<char, 32> buf{};
    std::snprintf(buf.data(), buf.size(), "%.1fs", s);
    return buf.data();
}

std::string with_detail(std::string line, const std::string& detail) {
    if (!detail.empty())
        line += " (" + detail + ")";
    return line;
}

} // namespace

const char* device_state_name(DeviceState s) {
    switch (s) {
    case DeviceState::Opening:
        return "opening";
    case DeviceState::Live:
        return "live";
    case DeviceState::Stalled:
        return "stalled";
    case DeviceState::Absent:
        return "absent";
    }
    return "?";
}

AudioHealthLog::AudioHealthLog(std::string label, LogFn log, const HealthLogParams& p)
    : label_(std::move(label)), log_(std::move(log)), p_(p) {}

void AudioHealthLog::log_transition(DeviceState from, DeviceState to, const std::string& detail,
                                    int64_t now_ns) {
    const std::string prefix = "audio " + label_ + ": ";
    if (to == DeviceState::Live) {
        if (!ever_live_) {
            log_(false, prefix + "live");
        } else {
            const double outage = static_cast<double>(now_ns - degraded_since_ns_) / kNsPerSec;
            log_(false, prefix + "live again after " + fmt_seconds(outage) + " " +
                            device_state_name(from));
        }
        return;
    }
    if (to == DeviceState::Stalled) {
        log_(true, with_detail(prefix + "no samples arriving; filling silence", detail));
        return;
    }
    if (to == DeviceState::Absent) {
        log_(true,
             with_detail(prefix + "device unavailable; filling silence and retrying", detail));
    }
}

void AudioHealthLog::on_state(DeviceState s, const std::string& detail, int64_t now_ns) {
    if (s == state_ || s == DeviceState::Opening)
        return;
    const DeviceState from = state_;
    state_ = s;
    const bool degraded = s == DeviceState::Stalled || s == DeviceState::Absent;
    if (degraded && degraded_since_ns_ < 0)
        degraded_since_ns_ = now_ns;

    const bool rate_limited =
        last_transition_log_ns_ >= 0 && now_ns - last_transition_log_ns_ < p_.min_transition_gap_ns;
    if (rate_limited && !(s == DeviceState::Live && !ever_live_)) {
        ++suppressed_transitions_;
    } else {
        log_transition(from, s, detail, now_ns);
        last_transition_log_ns_ = now_ns;
        logged_state_ = s;
    }
    if (s == DeviceState::Live) {
        ever_live_ = true;
        degraded_since_ns_ = -1;
    }
}

void AudioHealthLog::on_xrun() {
    ++xruns_;
}

void AudioHealthLog::log_settled_state(int64_t now_ns) {
    if (state_ == logged_state_ || state_ == DeviceState::Opening)
        return;
    if (now_ns - last_transition_log_ns_ < p_.min_transition_gap_ns)
        return;
    log_transition(logged_state_, state_, "settled after flapping", now_ns);
    last_transition_log_ns_ = now_ns;
    logged_state_ = state_;
}

void AudioHealthLog::tick(int64_t now_ns, int64_t silence_frames_total,
                          int64_t dropped_frames_total) {
    log_settled_state(now_ns);
    if (window_start_ns_ < 0) {
        window_start_ns_ = now_ns;
        silence_base_ = silence_frames_total;
        dropped_base_ = dropped_frames_total;
        return;
    }
    if (now_ns - window_start_ns_ < p_.summary_period_ns)
        return;
    const int64_t silence = silence_frames_total - silence_base_;
    const int64_t dropped = dropped_frames_total - dropped_base_;
    if (xruns_ > 0 || silence > 0 || dropped > 0 || suppressed_transitions_ > 0) {
        const double window = static_cast<double>(now_ns - window_start_ns_) / kNsPerSec;
        const double rate = p_.sample_rate > 0 ? p_.sample_rate : 48000;
        std::string line = "audio " + label_ + ": last " + fmt_seconds(window) + ": " +
                           std::to_string(xruns_) + " xruns, " +
                           fmt_seconds(static_cast<double>(silence) / rate) + " silence filled, " +
                           fmt_seconds(static_cast<double>(dropped) / rate) + " late audio dropped";
        if (suppressed_transitions_ > 0)
            line += ", " + std::to_string(suppressed_transitions_) +
                    " state changes not logged; now " + device_state_name(state_);
        log_(true, line);
    }
    window_start_ns_ = now_ns;
    silence_base_ = silence_frames_total;
    dropped_base_ = dropped_frames_total;
    xruns_ = 0;
    suppressed_transitions_ = 0;
}

} // namespace sink
