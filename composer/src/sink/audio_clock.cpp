#include "src/sink/audio_clock.hpp"

#include <cmath>
#include <numbers>

namespace sink {

namespace {
constexpr double kNsPerSec = 1e9;
constexpr double kRelockErrorNs = 50e6;
} // namespace

AudioClock::AudioClock(int sample_rate, double bandwidth_hz)
    : nominal_ns_per_frame_(kNsPerSec / (sample_rate > 0 ? sample_rate : 48000)),
      bandwidth_hz_(bandwidth_hz) {}

int64_t AudioClock::on_chunk(int64_t frames, int64_t end_ns) {
    if (frames <= 0)
        return static_cast<int64_t>(std::llround(next_start_ns_));
    const auto n = static_cast<double>(frames);
    const auto end = static_cast<double>(end_ns);
    const bool lost_lock =
        initialized_ && std::abs(end - (next_start_ns_ + n * ns_per_frame_)) > kRelockErrorNs;
    if (!initialized_ || lost_lock) {
        ns_per_frame_ = nominal_ns_per_frame_;
        next_start_ns_ = end;
        initialized_ = true;
        return static_cast<int64_t>(std::llround(end - n * ns_per_frame_));
    }
    const double start = next_start_ns_;
    const double predicted_end = start + n * ns_per_frame_;
    const double err = end - predicted_end;
    const double omega = 2.0 * std::numbers::pi * bandwidth_hz_ * (n * ns_per_frame_ / kNsPerSec);
    next_start_ns_ = predicted_end + std::numbers::sqrt2 * omega * err;
    ns_per_frame_ += omega * omega * err / n;
    return static_cast<int64_t>(std::llround(start));
}

double AudioClock::rate_hz() const {
    return ns_per_frame_ > 0 ? kNsPerSec / ns_per_frame_ : kNsPerSec / nominal_ns_per_frame_;
}

} // namespace sink
