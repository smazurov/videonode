#include "src/sink/audio_track.hpp"

#include <gtest/gtest.h>

#include <vector>

namespace {

constexpr int64_t kMs = 1'000'000;
constexpr int64_t kT0 = 10'000'000'000;
constexpr int64_t kChunk = 480;

struct Sim {
    sink::AudioTrack track;
    std::vector<sink::AudioPacket> packets;
    double device_rate = 48000.0;
    double t_ns = static_cast<double>(kT0);
    std::vector<int16_t> samples = std::vector<int16_t>(kChunk * 2, 100);

    explicit Sim(const sink::AudioTrackParams& p = {}) : track(p) { track.start(kT0); }

    void run_live(int64_t duration_ns, bool first_discontinuity = false) {
        const double end = t_ns + static_cast<double>(duration_ns);
        bool disc = first_discontinuity;
        while (t_ns < end) {
            t_ns += static_cast<double>(kChunk) * 1e9 / device_rate;
            track.push(samples, static_cast<int64_t>(t_ns), disc);
            disc = false;
            track.drain(static_cast<int64_t>(t_ns) + 2 * kMs, packets);
        }
    }

    void run_silent(int64_t duration_ns) {
        const double end = t_ns + static_cast<double>(duration_ns);
        while (t_ns < end) {
            t_ns += 10.0 * kMs;
            track.drain(static_cast<int64_t>(t_ns), packets);
        }
    }

    [[nodiscard]] int64_t frames_emitted() const {
        int64_t n = 0;
        for (const auto& p : packets)
            n += static_cast<int64_t>(p.data.size()) / 4;
        return n;
    }

    [[nodiscard]] bool pts_strictly_increasing() const {
        for (size_t i = 1; i < packets.size(); ++i) {
            if (packets[i].pts <= packets[i - 1].pts)
                return false;
        }
        return true;
    }
};

} // namespace

TEST(AudioTrack, SteadyStateIsGapless) {
    Sim s;
    s.run_live(5'000'000'000);
    EXPECT_EQ(s.track.silence_frames(), 0);
    EXPECT_EQ(s.track.dropped_frames(), 0);
    ASSERT_FALSE(s.packets.empty());
    EXPECT_NEAR(static_cast<double>(s.packets.front().pts), 0.0, 2.0);
    EXPECT_TRUE(s.pts_strictly_increasing());
}

TEST(AudioTrack, ClockDriftPassesThroughWithoutSilenceOrDrops) {
    for (const double ppm : {-500.0, 500.0}) {
        Sim s;
        s.device_rate = 48000.0 * (1 + ppm * 1e-6);
        s.run_live(120'000'000'000);
        EXPECT_EQ(s.track.silence_frames(), 0) << ppm;
        EXPECT_EQ(s.track.dropped_frames(), 0) << ppm;
        EXPECT_TRUE(s.pts_strictly_increasing()) << ppm;
        const auto& last = s.packets.back();
        const double wall_pts = (s.t_ns - static_cast<double>(kT0)) * 48000.0 / 1e9;
        EXPECT_NEAR(static_cast<double>(last.pts + kChunk), wall_pts, 48.0) << ppm;
    }
}

TEST(AudioTrack, SilenceCoversDeviceLossAfterFillDelay) {
    Sim s;
    s.run_live(1'000'000'000);
    const int64_t before = s.frames_emitted();
    s.run_silent(2'000'000'000);
    const int64_t silence = s.track.silence_frames();
    EXPECT_NEAR(static_cast<double>(silence), 2.0 * 48000 - 0.15 * 48000, 0.02 * 48000);
    EXPECT_EQ(s.frames_emitted() - before, silence);
    EXPECT_TRUE(s.pts_strictly_increasing());
}

TEST(AudioTrack, DeviceReturnResumesRealSamplesWithoutDrops) {
    Sim s;
    s.run_live(1'000'000'000);
    s.run_silent(3'000'000'000);
    s.run_live(1'000'000'000, true);
    EXPECT_EQ(s.track.dropped_frames(), 0);
    EXPECT_TRUE(s.pts_strictly_increasing());
    EXPECT_NEAR(static_cast<double>(s.frames_emitted()), 5.0 * 48000, 0.02 * 48000);
}

TEST(AudioTrack, LateBurstAfterFillIsDropped) {
    Sim s;
    s.run_live(1'000'000'000);
    const double stall_start = s.t_ns;
    s.run_silent(500'000'000);
    double t = stall_start;
    for (int k = 0; k < 40; ++k) {
        t += static_cast<double>(kChunk) * 1e9 / 48000.0;
        s.track.push(s.samples, static_cast<int64_t>(t), false);
    }
    s.track.drain(static_cast<int64_t>(s.t_ns), s.packets);
    EXPECT_GT(s.track.dropped_frames(), 0);
    EXPECT_TRUE(s.pts_strictly_increasing());
}

TEST(AudioTrack, PositiveOffsetDelaysAudio) {
    Sim s({.offset_ns = 100 * kMs});
    s.run_live(500'000'000);
    ASSERT_FALSE(s.packets.empty());
    EXPECT_EQ(s.packets.front().pts, 0);
    EXPECT_NEAR(static_cast<double>(s.track.silence_frames()), 4800.0, 2.0);
    EXPECT_TRUE(s.pts_strictly_increasing());
}

TEST(AudioTrack, NegativeOffsetTrimsPreRollAndAdvancesAudio) {
    Sim s({.offset_ns = -100 * kMs});
    s.run_live(500'000'000);
    ASSERT_FALSE(s.packets.empty());
    EXPECT_EQ(s.packets.front().pts, 0);
    EXPECT_EQ(s.track.silence_frames(), 0);
    EXPECT_TRUE(s.pts_strictly_increasing());
}

TEST(AudioTrack, PendingCapDropsOldest) {
    sink::AudioTrack t({.max_pending_frames = 4800});
    t.start(kT0);
    const std::vector<int16_t> samples(kChunk * 2, 1);
    for (int k = 1; k <= 50; ++k)
        t.push(samples, kT0 + k * 10 * kMs, false);
    EXPECT_GE(t.dropped_frames(), 50 * kChunk - 4800);
}

TEST(AudioTrack, PrerollBeforeT0IsNotCountedAsLate) {
    sink::AudioTrack t({});
    t.start(kT0);
    const std::vector<int16_t> samples(kChunk * 2, 1);
    for (int k = -20; k <= 5; ++k)
        t.push(samples, kT0 + k * 10 * kMs, false);
    std::vector<sink::AudioPacket> out;
    t.drain(kT0 + 60 * kMs, out);
    EXPECT_EQ(t.dropped_frames(), 0);
    ASSERT_FALSE(out.empty());
    EXPECT_EQ(out.front().pts, 0);
}

TEST(AudioTrack, IgnoresSamplesBeforeStart) {
    sink::AudioTrack t({});
    const std::vector<int16_t> samples(kChunk * 2, 1);
    t.push(samples, kT0, false);
    std::vector<sink::AudioPacket> out;
    t.drain(kT0 + kMs, out);
    EXPECT_TRUE(out.empty());
}
