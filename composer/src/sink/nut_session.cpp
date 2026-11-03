#include "src/sink/nut_session.hpp"

#include "src/common/log_levels.hpp"
#include "src/sink/alsa_capture.hpp"
#include "src/sink/audio_health.hpp"
#include "src/sink/audio_track.hpp"
#include "src/sink/mono_clock.hpp"
#include "src/sink/nut_writer.hpp"
#include "src/sink/nv12_pack.hpp"
#include "src/sink/video_link.hpp"
#include "src/sink/video_pacer.hpp"

#include <algorithm>
#include <memory>
#include <optional>
#include <poll.h>
#include <thread>
#include <unistd.h>

namespace sink {

namespace {

constexpr int64_t kNsPerMs = 1'000'000;
constexpr int64_t kAudioTickNs = 10 * kNsPerMs;
constexpr int64_t kVideoSummaryNs = 30'000'000'000;

struct AudioLane {
    std::unique_ptr<AlsaCapture> capture;
    AudioTrack track;
    AudioHealthLog health;
};

void log_line(bool warn, const std::string& line) {
    if (warn)
        vn::log::warn("videonode-sink: %s", line.c_str());
    else
        vn::log::info("videonode-sink: %s", line.c_str());
}

void consume_notify(int fd) {
    uint64_t val = 0;
    (void)::read(fd, &val, sizeof(val));
}

class NutSession {
  public:
    NutSession(const NutSessionArgs& a, std::atomic<bool>& running)
        : a_(a), running_(running),
          writer_([](std::span<const uint8_t> b) { return write_full(STDOUT_FILENO, b); }),
          link_({.socket_path = a.socket_path,
                 .settle_ms = a.settle_ms,
                 .initial_dial_timeout_ms = a.first_frame_timeout_s * 1000}) {}

    [[nodiscard]] int run();

  private:
    enum class Step : uint8_t { Continue, Exit, Fail };

    void build_lanes();
    [[nodiscard]] Step wait_first_frame();
    [[nodiscard]] bool start_stream(const scm_rights_source::OwnedFrameView& first);
    [[nodiscard]] Step tick();
    [[nodiscard]] bool pump_audio(int64_t now);
    [[nodiscard]] Step handle_new_frame(int64_t now);
    [[nodiscard]] bool write_video(int64_t slot);
    void poll_for_work(const std::shared_ptr<scm_rights_source::ScmRightsSource>& src,
                       int64_t now) const;
    void summarize_video(int64_t now);

    NutSessionArgs a_;
    std::atomic<bool>& running_;
    nut::Writer writer_;
    VideoLink link_;
    std::vector<std::unique_ptr<AudioLane>> lanes_;
    std::optional<VideoPacer> pacer_;
    AlignedScratch scratch_;
    std::span<const uint8_t> last_frame_;
    uint64_t last_idx_ = 0;
    uint64_t link_gen_ = 0;
    int width_ = 0;
    int height_ = 0;
    int64_t video_window_ns_ = 0;
    int64_t repeats_base_ = 0;
    int64_t drops_base_ = 0;
};

void NutSession::build_lanes() {
    const AudioTrackParams tp{.sample_rate = a_.audio_rate,
                              .channels = a_.audio_channels,
                              .offset_ns = static_cast<int64_t>(a_.audio_offset_ms) * kNsPerMs};
    const HealthLogParams hp{.sample_rate = a_.audio_rate};
    for (size_t k = 0; k < a_.audio_devices.size(); ++k) {
        const std::string& dev = a_.audio_devices[k];
        auto lane = std::make_unique<AudioLane>(AudioLane{
            .capture = std::make_unique<AlsaCapture>(AlsaCaptureParams{
                .device = dev, .sample_rate = a_.audio_rate, .channels = a_.audio_channels}),
            .track = AudioTrack(tp),
            .health = AudioHealthLog("[" + std::to_string(k) + "] " + dev, &log_line, hp)});
        lane->capture->start();
        lanes_.push_back(std::move(lane));
    }
}

NutSession::Step NutSession::wait_first_frame() {
    const int64_t deadline =
        mono_ns() + static_cast<int64_t>(a_.first_frame_timeout_s) * 1000 * kNsPerMs;
    while (running_.load()) {
        if (link_.initial_dial_failed()) {
            vn::log::fatal("videonode-sink: failed to dial %s (is the producer up?)",
                           a_.socket_path.c_str());
            return Step::Fail;
        }
        if (auto src = link_.current()) {
            auto f = src->latest_frame();
            if (f.fd.get() >= 0 && f.frame_idx != 0)
                return start_stream(f) ? Step::Continue : Step::Exit;
            poll_for_work(src, mono_ns());
        } else {
            std::this_thread::sleep_for(std::chrono::milliseconds(10));
        }
        if (!pump_audio(mono_ns()))
            return Step::Exit;
        if (mono_ns() > deadline) {
            vn::log::fatal("videonode-sink: timeout waiting for first frame on %s",
                           a_.socket_path.c_str());
            return Step::Fail;
        }
    }
    return Step::Exit;
}

bool NutSession::start_stream(const scm_rights_source::OwnedFrameView& first) {
    width_ = first.width;
    height_ = first.height;
    last_idx_ = first.frame_idx;
    link_gen_ = link_.generation();
    std::vector<nut::AudioParams> audio(
        lanes_.size(), {.sample_rate = a_.audio_rate, .channels = a_.audio_channels});
    if (!writer_.write_header({.width = width_, .height = height_, .fps = a_.fps}, audio))
        return false;
    vn::log::info("videonode-sink: streaming NUT (NV12 %dx%d @%d, %zu audio) from %s", width_,
                  height_, a_.fps, lanes_.size(), a_.socket_path.c_str());
    const int64_t t0 = mono_ns();
    pacer_.emplace(a_.fps, t0);
    video_window_ns_ = t0;
    for (auto& lane : lanes_)
        lane->track.start(t0);
    last_frame_ = pack_nv12(view_of(first), scratch_);
    const auto slot = pacer_->on_frame(t0);
    return last_frame_.empty() || !slot || write_video(*slot);
}

bool NutSession::write_video(int64_t slot) {
    if (last_frame_.empty())
        return true;
    return writer_.write_frame(nut::Writer::video_stream(), slot, last_frame_);
}

bool NutSession::pump_audio(int64_t now) {
    std::vector<CaptureEvent> events;
    std::vector<AudioPacket> packets;
    for (size_t k = 0; k < lanes_.size(); ++k) {
        AudioLane& lane = *lanes_[k];
        events.clear();
        lane.capture->drain_events(events);
        for (CaptureEvent& e : events) {
            if (e.kind == CaptureEvent::Kind::Samples)
                lane.track.push(e.samples, e.end_ns, e.discontinuity);
            else if (e.kind == CaptureEvent::Kind::State)
                lane.health.on_state(e.state, e.detail, now);
            else
                lane.health.on_xrun();
        }
        packets.clear();
        lane.track.drain(now, packets);
        for (const AudioPacket& p : packets) {
            if (!writer_.write_frame(nut::Writer::audio_stream(static_cast<uint32_t>(k)), p.pts,
                                     p.data))
                return false;
        }
        lane.health.tick(now, lane.track.silence_frames(), lane.track.dropped_frames());
    }
    return true;
}

NutSession::Step NutSession::handle_new_frame(int64_t now) {
    auto src = link_.current();
    if (!src)
        return Step::Continue;
    if (link_.generation() != link_gen_) {
        link_gen_ = link_.generation();
        last_idx_ = 0;
    }
    auto f = src->latest_frame();
    if (f.fd.get() < 0 || f.frame_idx == 0 || f.frame_idx == last_idx_)
        return Step::Continue;
    if (f.width != width_ || f.height != height_) {
        vn::log::warn("videonode-sink: source reconfigured %dx%d -> %dx%d; exiting so the "
                      "encoder rebuilds",
                      width_, height_, f.width, f.height);
        return Step::Exit;
    }
    last_idx_ = f.frame_idx;
    const auto slot = pacer_->on_frame(now);
    if (!slot)
        return Step::Continue;
    if (a_.verbose)
        vn::log::info("videonode-sink: frame_idx=%llu slot=%lld",
                      static_cast<unsigned long long>(f.frame_idx), static_cast<long long>(*slot));
    const auto packed = pack_nv12(view_of(f), scratch_);
    if (!packed.empty())
        last_frame_ = packed;
    return write_video(*slot) ? Step::Continue : Step::Exit;
}

void NutSession::poll_for_work(const std::shared_ptr<scm_rights_source::ScmRightsSource>& src,
                               int64_t now) const {
    int64_t wait_ns = kAudioTickNs;
    if (pacer_)
        wait_ns = std::min(wait_ns, std::max<int64_t>(pacer_->next_repeat_ns() - now, kNsPerMs));
    const int timeout_ms = static_cast<int>(std::max<int64_t>(wait_ns / kNsPerMs, 1));
    const int fd = src ? src->notify_fd() : -1;
    if (fd < 0) {
        std::this_thread::sleep_for(std::chrono::milliseconds(timeout_ms));
        return;
    }
    pollfd pfd{.fd = fd, .events = POLLIN, .revents = 0};
    if (::poll(&pfd, 1, timeout_ms) > 0 && (pfd.revents & POLLIN) != 0)
        consume_notify(fd);
}

void NutSession::summarize_video(int64_t now) {
    if (now - video_window_ns_ < kVideoSummaryNs)
        return;
    const int64_t repeats = pacer_->repeats() - repeats_base_;
    const int64_t drops = pacer_->drops() - drops_base_;
    if (repeats > 0 || drops > 0)
        vn::log::warn("videonode-sink: video last %llds: %lld frames repeated (upstream gaps), "
                      "%lld early frames dropped",
                      static_cast<long long>((now - video_window_ns_) / 1'000'000'000),
                      static_cast<long long>(repeats), static_cast<long long>(drops));
    video_window_ns_ = now;
    repeats_base_ = pacer_->repeats();
    drops_base_ = pacer_->drops();
}

NutSession::Step NutSession::tick() {
    poll_for_work(link_.current(), mono_ns());
    const int64_t now = mono_ns();
    if (!pump_audio(now))
        return Step::Exit;
    const Step s = handle_new_frame(now);
    if (s != Step::Continue)
        return s;
    if (const auto slot = pacer_->repeat_due(now); slot && !write_video(*slot))
        return Step::Exit;
    summarize_video(now);
    return Step::Continue;
}

int NutSession::run() {
    build_lanes();
    link_.start();
    Step s = wait_first_frame();
    while (s == Step::Continue && running_.load())
        s = tick();
    link_.stop();
    for (auto& lane : lanes_)
        lane->capture->stop();
    return s == Step::Fail ? 1 : 0;
}

} // namespace

int run_nut_session(const NutSessionArgs& a, std::atomic<bool>& running) {
    NutSession session(a, running);
    return session.run();
}

} // namespace sink
