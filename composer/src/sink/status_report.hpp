#pragma once

#include "src/sink/audio_health.hpp"

#include <cstdint>
#include <string>
#include <string_view>
#include <vector>

namespace sink {

// First line of every status connection; the daemon routes the connection
// to its sink parser on it (anything else is treated as ffmpeg -progress).
inline constexpr std::string_view kStatusHello = "sink_wire=1\n";

struct AudioInputStatus {
    std::string device;
    DeviceState state = DeviceState::Opening;
    int64_t state_age_ms = -1;
    std::string error;
    int64_t xruns = 0;
    int64_t silence_ms = 0;
    int64_t dropped_ms = 0;
    double rate_hz = 0;
};

struct SinkStatus {
    bool upstream_connected = false;
    int64_t repeated_frames = 0;
    int64_t dropped_frames = 0;
    std::vector<AudioInputStatus> audio;
};

// One key=value block terminated by "status=continue", in ffmpeg -progress framing.
[[nodiscard]] std::string format_status_block(const SinkStatus& s);

} // namespace sink
