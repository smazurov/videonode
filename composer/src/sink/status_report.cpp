#include "src/sink/status_report.hpp"

#include <array>
#include <cstdio>

namespace sink {

namespace {

std::string single_line(std::string v) {
    for (char& c : v) {
        if (c == '\n' || c == '\r')
            c = ' ';
    }
    return v;
}

void put(std::string& out, const std::string& key, const std::string& value) {
    out += key;
    out += '=';
    out += single_line(value);
    out += '\n';
}

std::string fmt_rate(double hz) {
    std::array<char, 32> buf{};
    std::snprintf(buf.data(), buf.size(), "%.2f", hz);
    return buf.data();
}

} // namespace

std::string format_status_block(const SinkStatus& s) {
    std::string out;
    put(out, "video.upstream", s.upstream_connected ? "connected" : "reconnecting");
    put(out, "video.repeated_frames", std::to_string(s.repeated_frames));
    put(out, "video.dropped_frames", std::to_string(s.dropped_frames));
    put(out, "audio.count", std::to_string(s.audio.size()));
    for (size_t k = 0; k < s.audio.size(); ++k) {
        const AudioInputStatus& a = s.audio[k];
        const std::string p = "audio." + std::to_string(k) + ".";
        put(out, p + "device", a.device);
        put(out, p + "state", device_state_name(a.state));
        if (a.state_age_ms >= 0)
            put(out, p + "state_age_ms", std::to_string(a.state_age_ms));
        if (!a.error.empty())
            put(out, p + "error", a.error);
        put(out, p + "xruns", std::to_string(a.xruns));
        put(out, p + "silence_ms", std::to_string(a.silence_ms));
        put(out, p + "dropped_ms", std::to_string(a.dropped_ms));
        if (a.rate_hz > 0)
            put(out, p + "rate_hz", fmt_rate(a.rate_hz));
    }
    out += "status=continue\n";
    return out;
}

} // namespace sink
