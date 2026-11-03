#pragma once

#include "src/sink/audio_health.hpp"

#include <atomic>
#include <cstdint>
#include <memory>
#include <mutex>
#include <string>
#include <thread>
#include <vector>

struct _snd_pcm;

namespace sink {

struct CaptureEvent {
    enum class Kind : uint8_t { Samples, State, Xrun };
    Kind kind = Kind::Samples;
    std::vector<int16_t> samples;
    int64_t end_ns = 0;
    bool discontinuity = false;
    DeviceState state = DeviceState::Opening;
    std::string detail;

    [[nodiscard]] static CaptureEvent make_samples(std::vector<int16_t> samples, int64_t end_ns,
                                                   bool discontinuity);
    [[nodiscard]] static CaptureEvent make_state(DeviceState s, std::string detail);
    [[nodiscard]] static CaptureEvent make_xrun();
};

struct AlsaCaptureParams {
    std::string device;
    int sample_rate = 48000;
    int channels = 2;
    int period_frames = 480;
    int64_t stall_ns = 500'000'000;
    int64_t reopen_after_stall_ns = 2'000'000'000;
    int64_t retry_ns = 500'000'000;
    int max_xruns_per_sec = 20;
    int64_t max_queued_frames = 96000;
};

// Owns one ALSA capture device on its own thread: reads it continuously,
// recovers xruns, reopens after unplug/stall, and never blocks on the consumer.
class AlsaCapture {
  public:
    explicit AlsaCapture(AlsaCaptureParams p);
    ~AlsaCapture();
    AlsaCapture(const AlsaCapture&) = delete;
    AlsaCapture& operator=(const AlsaCapture&) = delete;

    void start();
    void stop();
    void drain_events(std::vector<CaptureEvent>& out);

  private:
    enum class ReadOutcome : uint8_t { Data, Idle, Xrun, Lost };

    struct PcmCloser {
        void operator()(_snd_pcm* pcm) const;
    };
    using PcmHandle = std::unique_ptr<_snd_pcm, PcmCloser>;

    void thread_main_();
    [[nodiscard]] bool open_(std::string& err);
    void close_(DeviceState next, const std::string& detail);
    [[nodiscard]] ReadOutcome read_once_(std::vector<int16_t>& buf);
    [[nodiscard]] bool recover_xrun_();
    void handle_idle_();
    void push_(CaptureEvent e);
    void set_state_(DeviceState s, const std::string& detail);
    void sleep_interruptible_(int64_t ns);

    AlsaCaptureParams p_;
    PcmHandle pcm_;
    std::thread thread_;
    std::atomic<bool> running_{false};
    DeviceState state_ = DeviceState::Opening;
    bool discontinuity_next_ = true;
    int64_t idle_ns_ = 0;
    int64_t xrun_window_start_ns_ = 0;
    int xruns_in_window_ = 0;
    int last_err_ = 0;

    std::mutex mu_;
    std::vector<CaptureEvent> queue_;
    int64_t queued_frames_ = 0;
};

} // namespace sink
