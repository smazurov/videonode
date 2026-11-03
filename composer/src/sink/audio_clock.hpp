#pragma once

#include <cstdint>

namespace sink {

// Delay-locked loop (Adriaensen, "Using a DLL to filter time") over capture
// timestamps: smooths read jitter and tracks the device's true sample rate.
class AudioClock {
  public:
    explicit AudioClock(int sample_rate, double bandwidth_hz = 0.1);

    void reset() { initialized_ = false; }

    // Feed `frames` samples whose last sample ended at end_ns; returns the
    // smoothed capture time of the chunk's first sample.
    [[nodiscard]] int64_t on_chunk(int64_t frames, int64_t end_ns);

    [[nodiscard]] double rate_hz() const;
    [[nodiscard]] bool initialized() const { return initialized_; }

  private:
    double nominal_ns_per_frame_;
    double bandwidth_hz_;
    double ns_per_frame_ = 0;
    double next_start_ns_ = 0;
    bool initialized_ = false;
};

} // namespace sink
