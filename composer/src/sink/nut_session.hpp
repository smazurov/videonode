#pragma once

#include <atomic>
#include <string>
#include <vector>

namespace sink {

struct NutSessionArgs {
    std::string socket_path;
    int fps = 30;
    std::vector<std::string> audio_devices;
    int audio_rate = 48000;
    int audio_channels = 2;
    int audio_offset_ms = 0;
    int settle_ms = 200;
    int first_frame_timeout_s = 30;
    bool verbose = false;
};

// Muxes paced NV12 video plus N gapless PCM tracks into one NUT stream on stdout.
[[nodiscard]] int run_nut_session(const NutSessionArgs& a, std::atomic<bool>& running);

} // namespace sink
