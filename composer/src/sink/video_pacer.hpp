#pragma once

#include <cstdint>
#include <optional>

namespace sink {

// Maps frame arrivals onto a 1/fps slot grid anchored at t0, and repeats the last
// frame when upstream goes quiet so the encoder's video timeline never stops.
class VideoPacer {
  public:
    VideoPacer(int fps, int64_t t0_ns);

    [[nodiscard]] std::optional<int64_t> on_frame(int64_t now_ns);
    [[nodiscard]] std::optional<int64_t> repeat_due(int64_t now_ns);
    [[nodiscard]] int64_t next_repeat_ns() const;
    [[nodiscard]] int64_t last_slot() const { return last_slot_; }
    [[nodiscard]] int64_t repeats() const { return repeats_; }
    [[nodiscard]] int64_t drops() const { return drops_; }

  private:
    [[nodiscard]] int64_t slot_floor(int64_t now_ns) const;
    [[nodiscard]] int64_t slot_round(int64_t now_ns) const;
    [[nodiscard]] int64_t slot_start_ns(int64_t slot) const;

    int fps_;
    int64_t t0_ns_;
    int64_t last_slot_ = -1;
    int64_t repeats_ = 0;
    int64_t drops_ = 0;
};

} // namespace sink
