#include "src/capture/signal_activity.hpp"

#include <algorithm>

namespace signal_activity {

namespace {
constexpr int kTargetSamplesPerAxis = 64;
constexpr uint64_t kFnvOffset = 1469598103934665603ULL;
constexpr uint64_t kFnvPrime = 1099511628211ULL;
} // namespace

LumaStats compute_luma_stats(const LumaView& v) {
    if (v.width <= 0 || v.height <= 0 || v.row_pitch <= 0 || v.pixel_stride <= 0)
        return {};
    const int step_x = std::max(1, v.width / kTargetSamplesPerAxis);
    const int step_y = std::max(1, v.height / kTargetSamplesPerAxis);

    uint64_t sum = 0;
    uint64_t sum_sq = 0;
    uint64_t count = 0;
    uint64_t hash = kFnvOffset;
    for (int y = 0; y < v.height; y += step_y) {
        const size_t row = static_cast<size_t>(y) * v.row_pitch + v.sample_offset;
        for (int x = 0; x < v.width; x += step_x) {
            const size_t idx = row + static_cast<size_t>(x) * v.pixel_stride;
            if (idx >= v.data.size())
                return {};
            const uint8_t s = v.data[idx];
            sum += s;
            sum_sq += static_cast<uint64_t>(s) * s;
            hash = (hash ^ s) * kFnvPrime;
            ++count;
        }
    }
    if (count == 0)
        return {};
    const double mean = static_cast<double>(sum) / static_cast<double>(count);
    const double variance = static_cast<double>(sum_sq) / static_cast<double>(count) - mean * mean;
    return {.variance = variance < 0.0 ? 0.0 : variance, .signature = hash, .valid = true};
}

bool Detector::update(const LumaStats& s) {
    if (!s.valid) {
        dead_streak_ = 0;
        have_prev_ = false;
        return false;
    }
    const bool frozen = have_prev_ && s.signature == prev_signature_;
    const bool flat = s.variance <= flat_variance_;
    prev_signature_ = s.signature;
    have_prev_ = true;
    if (flat && frozen)
        dead_streak_ = std::min(dead_streak_ + 1, dead_frames_);
    else
        dead_streak_ = 0;
    return dead_streak_ >= dead_frames_;
}

void Detector::reset() {
    have_prev_ = false;
    dead_streak_ = 0;
}

} // namespace signal_activity
