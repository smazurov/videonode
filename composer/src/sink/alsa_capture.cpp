#include "src/sink/alsa_capture.hpp"

#include "src/sink/mono_clock.hpp"

#include <alsa/asoundlib.h>

#include <cerrno>
#include <chrono>
#include <utility>

namespace sink {

namespace {

constexpr int kWaitMs = 100;
constexpr int64_t kNsPerMs = 1'000'000;
constexpr int64_t kNsPerSec = 1'000'000'000;
constexpr unsigned kBufferLatencyUs = 500'000;

// alsa-lib prints every dsnoop recovery attempt to stderr (hundreds per unplug); AudioHealthLog
// reports instead.
extern "C" void quiet_alsa_errors(const char* /*file*/, int /*line*/, const char* /*fn*/,
                                  int /*err*/, const char* /*fmt*/, ...) {}

void install_quiet_alsa_errors() {
    static const bool installed = [] {
        snd_lib_error_set_handler(&quiet_alsa_errors);
        return true;
    }();
    (void)installed;
}

} // namespace

CaptureEvent CaptureEvent::make_samples(std::vector<int16_t> samples, int64_t end_ns,
                                        bool discontinuity) {
    CaptureEvent e;
    e.samples = std::move(samples);
    e.end_ns = end_ns;
    e.discontinuity = discontinuity;
    return e;
}

CaptureEvent CaptureEvent::make_state(DeviceState s, std::string detail) {
    CaptureEvent e;
    e.kind = Kind::State;
    e.state = s;
    e.detail = std::move(detail);
    return e;
}

CaptureEvent CaptureEvent::make_xrun() {
    CaptureEvent e;
    e.kind = Kind::Xrun;
    return e;
}

void AlsaCapture::PcmCloser::operator()(_snd_pcm* pcm) const {
    if (pcm != nullptr)
        snd_pcm_close(pcm);
}

AlsaCapture::AlsaCapture(AlsaCaptureParams p) : p_(std::move(p)) {}

AlsaCapture::~AlsaCapture() {
    stop();
}

void AlsaCapture::start() {
    if (running_.exchange(true))
        return;
    install_quiet_alsa_errors();
    thread_ = std::thread([this] { thread_main_(); });
}

void AlsaCapture::stop() {
    running_.store(false);
    if (thread_.joinable())
        thread_.join();
    pcm_.reset();
}

void AlsaCapture::drain_events(std::vector<CaptureEvent>& out) {
    std::lock_guard<std::mutex> lk(mu_);
    for (CaptureEvent& e : queue_)
        out.push_back(std::move(e));
    queue_.clear();
    queued_frames_ = 0;
}

void AlsaCapture::push_(CaptureEvent e) {
    std::lock_guard<std::mutex> lk(mu_);
    if (e.kind == CaptureEvent::Kind::Samples) {
        const int64_t frames = static_cast<int64_t>(e.samples.size()) / p_.channels;
        queued_frames_ += frames;
        while (queued_frames_ > p_.max_queued_frames) {
            auto it = queue_.begin();
            while (it != queue_.end() && it->kind != CaptureEvent::Kind::Samples)
                ++it;
            if (it == queue_.end())
                break;
            queued_frames_ -= static_cast<int64_t>(it->samples.size()) / p_.channels;
            queue_.erase(it);
            e.discontinuity = true;
        }
    }
    queue_.push_back(std::move(e));
}

void AlsaCapture::set_state_(DeviceState s, const std::string& detail) {
    if (s == state_)
        return;
    state_ = s;
    push_(CaptureEvent::make_state(s, detail));
}

void AlsaCapture::sleep_interruptible_(int64_t ns) {
    const int64_t deadline = mono_ns() + ns;
    while (running_.load() && mono_ns() < deadline)
        std::this_thread::sleep_for(std::chrono::milliseconds(20));
}

bool AlsaCapture::open_(std::string& err) {
    snd_pcm_t* raw = nullptr;
    int rc = snd_pcm_open(&raw, p_.device.c_str(), SND_PCM_STREAM_CAPTURE, SND_PCM_NONBLOCK);
    if (rc < 0) {
        err = snd_strerror(rc);
        return false;
    }
    PcmHandle pcm(raw);
    rc = snd_pcm_set_params(pcm.get(), SND_PCM_FORMAT_S16_LE, SND_PCM_ACCESS_RW_INTERLEAVED,
                            static_cast<unsigned>(p_.channels),
                            static_cast<unsigned>(p_.sample_rate), 1, kBufferLatencyUs);
    if (rc >= 0)
        rc = snd_pcm_start(pcm.get());
    if (rc < 0) {
        err = snd_strerror(rc);
        return false;
    }
    pcm_ = std::move(pcm);
    discontinuity_next_ = true;
    idle_ns_ = 0;
    xruns_in_window_ = 0;
    return true;
}

void AlsaCapture::close_(DeviceState next, const std::string& detail) {
    pcm_.reset();
    set_state_(next, detail);
}

bool AlsaCapture::recover_xrun_() {
    push_(CaptureEvent::make_xrun());
    discontinuity_next_ = true;
    const int64_t now = mono_ns();
    if (now - xrun_window_start_ns_ > kNsPerSec) {
        xrun_window_start_ns_ = now;
        xruns_in_window_ = 0;
    }
    if (++xruns_in_window_ > p_.max_xruns_per_sec)
        return false;
    return snd_pcm_prepare(pcm_.get()) >= 0 && snd_pcm_start(pcm_.get()) >= 0;
}

AlsaCapture::ReadOutcome AlsaCapture::read_once_(std::vector<int16_t>& buf) {
    const int w = snd_pcm_wait(pcm_.get(), kWaitMs);
    if (w == 0)
        return ReadOutcome::Idle;
    if (w == -EPIPE)
        return ReadOutcome::Xrun;
    if (w < 0) {
        last_err_ = w;
        return ReadOutcome::Lost;
    }
    const snd_pcm_sframes_t got =
        snd_pcm_readi(pcm_.get(), buf.data(), static_cast<snd_pcm_uframes_t>(p_.period_frames));
    if (got == -EAGAIN)
        return ReadOutcome::Idle;
    if (got == -EPIPE)
        return ReadOutcome::Xrun;
    if (got < 0) {
        last_err_ = static_cast<int>(got);
        return snd_pcm_recover(pcm_.get(), last_err_, 1) >= 0 ? ReadOutcome::Xrun
                                                              : ReadOutcome::Lost;
    }
    snd_pcm_sframes_t delay = 0;
    if (snd_pcm_delay(pcm_.get(), &delay) < 0)
        delay = 0;
    const int64_t end_ns = mono_ns() - static_cast<int64_t>(delay) * kNsPerSec / p_.sample_rate;
    const auto n = static_cast<size_t>(got) * static_cast<size_t>(p_.channels);
    push_(CaptureEvent::make_samples(
        std::vector<int16_t>(buf.begin(), buf.begin() + static_cast<std::ptrdiff_t>(n)), end_ns,
        discontinuity_next_));
    discontinuity_next_ = false;
    idle_ns_ = 0;
    set_state_(DeviceState::Live, {});
    return ReadOutcome::Data;
}

void AlsaCapture::handle_idle_() {
    idle_ns_ += kWaitMs * kNsPerMs;
    if (idle_ns_ >= p_.reopen_after_stall_ns) {
        close_(DeviceState::Stalled,
               "reopening after " + std::to_string(idle_ns_ / kNsPerMs) + " ms without samples");
        return;
    }
    if (idle_ns_ >= p_.stall_ns)
        set_state_(DeviceState::Stalled, p_.device);
}

void AlsaCapture::thread_main_() {
    std::vector<int16_t> buf(static_cast<size_t>(p_.period_frames) *
                             static_cast<size_t>(p_.channels));
    while (running_.load()) {
        if (!pcm_) {
            std::string err;
            if (!open_(err)) {
                set_state_(DeviceState::Absent, p_.device + ": " + err);
                sleep_interruptible_(p_.retry_ns);
                continue;
            }
        }
        switch (read_once_(buf)) {
        case ReadOutcome::Data:
            break;
        case ReadOutcome::Idle:
            handle_idle_();
            break;
        case ReadOutcome::Xrun:
            if (!recover_xrun_())
                close_(DeviceState::Absent, p_.device + ": capture would not restart");
            break;
        case ReadOutcome::Lost:
            close_(DeviceState::Absent, p_.device + ": " + snd_strerror(last_err_));
            break;
        }
    }
}

} // namespace sink
