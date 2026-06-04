// signal_activity — content-based "no signal" detector for capture sources
// that keep streaming when their input is dead (e.g. the MACROSILICON MS2130
// UVC dongle emits a constant flat video-black frame with no V4L2 signal
// status). A frame is "dead" when its luma is near-flat (low variance) AND
// byte-identical to the previous frame; sustained dead frames mean no signal.
// Live input — even a dark scene — carries sensor/encode noise, so it varies.

#pragma once

#include <cstddef>
#include <cstdint>
#include <span>

namespace signal_activity {

// A view of one frame's luma samples. pixel_stride/sample_offset let one
// sampler walk packed YUYV/UYVY (stride 2, offset 0/1) and planar NV12
// (stride 1, offset 0) without copying.
struct LumaView {
    std::span<const uint8_t> data;
    int width = 0;
    int height = 0;
    int row_pitch = 0;     // bytes per row
    int pixel_stride = 1;  // bytes between luma samples within a row
    int sample_offset = 0; // byte offset of the first luma sample in a row
};

struct LumaStats {
    double variance = 0.0;
    uint64_t signature = 0; // cheap hash for frame-to-frame freeze detection
    bool valid = false;
};

// Subsample the luma on a coarse grid and return its variance + a freeze hash.
[[nodiscard]] LumaStats compute_luma_stats(const LumaView& v);

// Tracks dead-frame streaks with asymmetric debounce: a source is declared
// "no signal" only after dead_frames consecutive flat+frozen frames, and
// recovers to live on the first frame that varies or moves.
class Detector {
  public:
    void set_thresholds(double flat_variance, int dead_frames) {
        flat_variance_ = flat_variance;
        dead_frames_ = dead_frames;
    }

    // Feed one frame's stats. Returns true while the source reads "no signal".
    [[nodiscard]] bool update(const LumaStats& s);

    void reset();

  private:
    double flat_variance_ = 6.0;
    int dead_frames_ = 30;
    uint64_t prev_signature_ = 0;
    bool have_prev_ = false;
    int dead_streak_ = 0;
};

} // namespace signal_activity
