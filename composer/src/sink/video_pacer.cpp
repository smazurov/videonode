#include "src/sink/video_pacer.hpp"

#include <algorithm>

namespace sink {

namespace {
constexpr int64_t kNsPerSec = 1'000'000'000;
}

VideoPacer::VideoPacer(int fps, int64_t t0_ns) : fps_(fps > 0 ? fps : 30), t0_ns_(t0_ns) {}

int64_t VideoPacer::slot_floor(int64_t now_ns) const {
    const int64_t dt = std::max<int64_t>(0, now_ns - t0_ns_);
    return dt * fps_ / kNsPerSec;
}

int64_t VideoPacer::slot_round(int64_t now_ns) const {
    const int64_t dt = std::max<int64_t>(0, now_ns - t0_ns_);
    return (dt * fps_ + kNsPerSec / 2) / kNsPerSec;
}

int64_t VideoPacer::slot_start_ns(int64_t slot) const {
    return t0_ns_ + slot * kNsPerSec / fps_;
}

std::optional<int64_t> VideoPacer::on_frame(int64_t now_ns) {
    const int64_t slot = std::max(slot_round(now_ns), last_slot_ + 1);
    if (slot > slot_floor(now_ns) + 1) {
        ++drops_;
        return std::nullopt;
    }
    last_slot_ = slot;
    return slot;
}

std::optional<int64_t> VideoPacer::repeat_due(int64_t now_ns) {
    if (last_slot_ < 0 || slot_floor(now_ns) < last_slot_ + 2)
        return std::nullopt;
    ++repeats_;
    last_slot_ = std::max(last_slot_ + 1, slot_floor(now_ns) - 1);
    return last_slot_;
}

int64_t VideoPacer::next_repeat_ns() const {
    return slot_start_ns(std::max<int64_t>(last_slot_, 0) + 2);
}

} // namespace sink
