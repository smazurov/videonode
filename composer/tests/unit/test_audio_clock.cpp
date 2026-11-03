#include "src/sink/audio_clock.hpp"

#include <gtest/gtest.h>

#include <cmath>
#include <cstdlib>

namespace {

struct Feed {
    double true_rate;
    int64_t chunk;
    double t_ns = 5e9;
    int64_t i = 0;

    int64_t next_end(int64_t jitter_ns) {
        t_ns += static_cast<double>(chunk) * 1e9 / true_rate;
        ++i;
        return static_cast<int64_t>(t_ns) + jitter_ns;
    }
};

} // namespace

TEST(AudioClock, TracksFastDeviceDespiteJitter) {
    sink::AudioClock c(48000);
    Feed f{.true_rate = 48000.0 * (1 + 200e-6), .chunk = 2048};
    double rate_sum = 0;
    double worst_phase_ns = 0;
    for (int k = 0; k < 4000; ++k) {
        const double true_start = f.t_ns;
        const int64_t start = c.on_chunk(f.chunk, f.next_end(((k * 7919) % 2001 - 1000) * 1000));
        if (k >= 2000) {
            rate_sum += c.rate_hz();
            worst_phase_ns =
                std::max(worst_phase_ns, std::abs(static_cast<double>(start) - true_start));
        }
    }
    EXPECT_NEAR(rate_sum / 2000, f.true_rate, 1.0);
    EXPECT_LT(worst_phase_ns, 1e6);
}

TEST(AudioClock, SmoothsJitterIntoNearlyUniformSteps) {
    sink::AudioClock c(48000);
    Feed f{.true_rate = 48000.0, .chunk = 480};
    int64_t prev = c.on_chunk(f.chunk, f.next_end(0));
    double worst = 0;
    for (int k = 1; k < 3000; ++k) {
        const int64_t start = c.on_chunk(f.chunk, f.next_end(((k * 104729) % 4001 - 2000) * 1000));
        if (k > 1500)
            worst = std::max(worst, std::abs(static_cast<double>(start - prev) - 1e7));
        prev = start;
    }
    EXPECT_LT(worst, 100'000.0);
}

TEST(AudioClock, RelocksAfterLargeJump) {
    sink::AudioClock c(48000);
    Feed f{.true_rate = 48000.0, .chunk = 480};
    for (int k = 0; k < 100; ++k)
        (void)c.on_chunk(f.chunk, f.next_end(0));
    f.t_ns += 3e9;
    const int64_t end = f.next_end(0);
    const int64_t start = c.on_chunk(f.chunk, end);
    EXPECT_NEAR(static_cast<double>(end - start), 1e7, 1e5);
}

TEST(AudioClock, ResetReanchors) {
    sink::AudioClock c(48000);
    (void)c.on_chunk(480, 1'000'000'000);
    c.reset();
    EXPECT_FALSE(c.initialized());
    EXPECT_EQ(c.on_chunk(480, 9'000'000'000), 9'000'000'000 - 10'000'000);
}
