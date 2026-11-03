#pragma once

#include <cstdint>
#include <functional>
#include <string>

namespace sink {

enum class DeviceState : uint8_t { Opening, Live, Stalled, Absent };

[[nodiscard]] const char* device_state_name(DeviceState s);

struct HealthLogParams {
    int sample_rate = 48000;
    int64_t summary_period_ns = 30'000'000'000;
    int64_t min_transition_gap_ns = 5'000'000'000;
};

// Rate-limited audio logging: one line per state transition (flapping beyond
// one per min_transition_gap is folded into the summary), plus one summary line
// per period covering xruns / silence filled / late audio dropped, only when nonzero.
class AudioHealthLog {
  public:
    using LogFn = std::function<void(bool warn, const std::string& line)>;

    AudioHealthLog(std::string label, LogFn log, const HealthLogParams& p = {});

    void on_state(DeviceState s, const std::string& detail, int64_t now_ns);
    void on_xrun();
    void tick(int64_t now_ns, int64_t silence_frames_total, int64_t dropped_frames_total);

    [[nodiscard]] DeviceState state() const { return state_; }

  private:
    void log_transition(DeviceState from, DeviceState to, const std::string& detail,
                        int64_t now_ns);
    void log_settled_state(int64_t now_ns);

    std::string label_;
    LogFn log_;
    HealthLogParams p_;
    DeviceState state_ = DeviceState::Opening;
    DeviceState logged_state_ = DeviceState::Opening;
    bool ever_live_ = false;
    int64_t degraded_since_ns_ = -1;
    int64_t last_transition_log_ns_ = -1;
    int64_t window_start_ns_ = -1;
    int64_t suppressed_transitions_ = 0;
    int64_t xruns_ = 0;
    int64_t silence_base_ = 0;
    int64_t dropped_base_ = 0;
};

} // namespace sink
