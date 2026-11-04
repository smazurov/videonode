#include "src/sink/status_report.hpp"

#include <gtest/gtest.h>

#include <string>

namespace {

bool has_line(const std::string& block, const std::string& line) {
    return block.find(line + "\n") != std::string::npos;
}

} // namespace

TEST(StatusReport, FormatsVideoAndAudioInputs) {
    sink::SinkStatus s;
    s.upstream_connected = true;
    s.repeated_frames = 12;
    s.audio.push_back({.device = "vncap:CARD=Lyra,DEV=0",
                       .state = sink::DeviceState::Absent,
                       .state_age_ms = 9700,
                       .error = "vncap:CARD=Lyra,DEV=0: No such device",
                       .xruns = 1,
                       .silence_ms = 9400,
                       .dropped_ms = 0,
                       .rate_hz = 48000.94});
    const std::string b = sink::format_status_block(s);
    for (const char* line :
         {"video.upstream=connected", "video.repeated_frames=12", "video.dropped_frames=0",
          "audio.count=1", "audio.0.device=vncap:CARD=Lyra,DEV=0", "audio.0.state=absent",
          "audio.0.state_age_ms=9700", "audio.0.error=vncap:CARD=Lyra,DEV=0: No such device",
          "audio.0.xruns=1", "audio.0.silence_ms=9400", "audio.0.dropped_ms=0",
          "audio.0.rate_hz=48000.94"}) {
        EXPECT_TRUE(has_line(b, line)) << line << "\n" << b;
    }
    EXPECT_EQ(b.substr(b.size() - 16), "status=continue\n");
}

TEST(StatusReport, OmitsUnknownFieldsAndFlattensNewlines) {
    sink::SinkStatus s;
    s.audio.push_back({.device = "hw:0,0", .error = "line one\nline two"});
    const std::string b = sink::format_status_block(s);
    EXPECT_TRUE(has_line(b, "video.upstream=reconnecting"));
    EXPECT_TRUE(has_line(b, "audio.0.state=opening"));
    EXPECT_TRUE(has_line(b, "audio.0.error=line one line two"));
    EXPECT_EQ(b.find("state_age_ms"), std::string::npos);
    EXPECT_EQ(b.find("rate_hz"), std::string::npos);
}

TEST(StatusReport, HelloIsTheWireVersionLine) {
    EXPECT_EQ(sink::kStatusHello, "sink_wire=1\n");
}
