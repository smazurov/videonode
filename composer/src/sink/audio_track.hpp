#pragma once

#include "src/sink/audio_clock.hpp"

#include <cstdint>
#include <deque>
#include <span>
#include <vector>

namespace sink {

struct AudioPacket {
    int64_t pts = 0;
    std::vector<uint8_t> data;
};

struct AudioTrackParams {
    int sample_rate = 48000;
    int channels = 2;
    int64_t offset_ns = 0;
    int64_t fill_after_ns = 150'000'000;
    int64_t tolerance_frames = 960;
    int64_t silence_packet_frames = 960;
    int64_t max_pending_frames = 96000;
};

// Turns timestamped capture chunks into a gapless PCM packet stream on the
// output timeline: real samples keep their smoothed capture pts (ffmpeg's
// aresample absorbs clock drift), and silence covers any span the device misses.
class AudioTrack {
  public:
    explicit AudioTrack(const AudioTrackParams& p);

    void start(int64_t t0_ns);
    void push(std::span<const int16_t> samples, int64_t end_ns, bool discontinuity);
    void drain(int64_t now_ns, std::vector<AudioPacket>& out);

    [[nodiscard]] int64_t silence_frames() const { return silence_frames_; }
    [[nodiscard]] int64_t dropped_frames() const { return dropped_frames_; }
    [[nodiscard]] bool started() const { return started_; }

  private:
    struct Pending {
        int64_t pts = 0;
        std::vector<int16_t> samples;
    };

    [[nodiscard]] int64_t frames_of(const Pending& p) const;
    [[nodiscard]] int64_t pts_for_ns(int64_t ns) const;
    void emit_silence(int64_t until_pts, std::vector<AudioPacket>& out);
    void emit_chunk(Pending& c, std::vector<AudioPacket>& out);
    void trim_head(Pending& c, int64_t frames, bool late);
    void enforce_pending_cap();

    AudioTrackParams p_;
    AudioClock clock_;
    std::deque<Pending> pending_;
    int64_t pending_frames_ = 0;
    int64_t t0_ns_ = 0;
    int64_t out_pts_ = 0;
    int64_t last_pts_ = -1;
    int64_t silence_frames_ = 0;
    int64_t dropped_frames_ = 0;
    bool started_ = false;
};

} // namespace sink
