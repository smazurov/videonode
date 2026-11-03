#include "src/sink/video_link.hpp"

#include "src/common/log_levels.hpp"
#include "src/sink/mono_clock.hpp"

#include <chrono>
#include <utility>

namespace sink {

namespace {
constexpr int kDialAttemptMs = 2000;
constexpr int kLivenessPollMs = 50;
constexpr int64_t kNsPerSec = 1'000'000'000;
constexpr int64_t kOutageReminderNs = 60 * kNsPerSec;
} // namespace

scm_rights_source::FrameView view_of(const scm_rights_source::OwnedFrameView& f) {
    scm_rights_source::FrameView v;
    v.fd = f.fd.get();
    v.plane1_fd = f.plane1_fd.get();
    v.width = f.width;
    v.height = f.height;
    v.plane0_pitch = f.plane0_pitch;
    v.plane0_offset = f.plane0_offset;
    v.plane1_pitch = f.plane1_pitch;
    v.plane1_offset = f.plane1_offset;
    v.format = f.format;
    v.frame_idx = f.frame_idx;
    return v;
}

VideoLink::VideoLink(Params p) : p_(std::move(p)) {}

VideoLink::~VideoLink() {
    stop();
}

void VideoLink::start() {
    if (running_.exchange(true))
        return;
    thread_ = std::thread([this] { thread_main_(); });
}

void VideoLink::stop() {
    running_.store(false);
    if (thread_.joinable())
        thread_.join();
    publish_(nullptr);
}

std::shared_ptr<scm_rights_source::ScmRightsSource> VideoLink::current() const {
    std::lock_guard<std::mutex> lk(mu_);
    return current_;
}

void VideoLink::publish_(std::shared_ptr<scm_rights_source::ScmRightsSource> src) {
    std::lock_guard<std::mutex> lk(mu_);
    current_ = std::move(src);
    generation_.fetch_add(1);
}

std::shared_ptr<scm_rights_source::ScmRightsSource> VideoLink::dial_() {
    auto src = std::make_shared<scm_rights_source::ScmRightsSource>();
    scm_rights_source::InitParams ip;
    ip.socket_path = p_.socket_path;
    ip.dial = true;
    ip.dial_timeout_ms = kDialAttemptMs;
    ip.quiet_dial_failure = failed_dials_ > 0;
    if (!src->init(ip) || !src->start()) {
        ++failed_dials_;
        return nullptr;
    }
    failed_dials_ = 0;
    return src;
}

void VideoLink::note_still_down_(int64_t now) {
    if (outage_start_ns_ < 0 || now - last_outage_log_ns_ < kOutageReminderNs)
        return;
    vn::log::warn("videonode-sink: %s unreachable for %llds; repeating last frame",
                  p_.socket_path.c_str(),
                  static_cast<long long>((now - outage_start_ns_) / kNsPerSec));
    last_outage_log_ns_ = now;
}

void VideoLink::note_connected_(int64_t now) {
    if (outage_start_ns_ >= 0)
        vn::log::info("videonode-sink: reconnected to %s after %.1fs", p_.socket_path.c_str(),
                      static_cast<double>(now - outage_start_ns_) / static_cast<double>(kNsPerSec));
    outage_start_ns_ = -1;
}

void VideoLink::thread_main_() {
    bool ever_connected = false;
    const int64_t initial_deadline =
        mono_ns() + static_cast<int64_t>(p_.initial_dial_timeout_ms) * 1'000'000;
    while (running_.load()) {
        auto src = dial_();
        if (!src) {
            if (!ever_connected && mono_ns() > initial_deadline) {
                initial_dial_failed_.store(true);
                return;
            }
            note_still_down_(mono_ns());
            continue;
        }
        ever_connected = true;
        note_connected_(mono_ns());
        std::this_thread::sleep_for(std::chrono::milliseconds(p_.settle_ms));
        publish_(src);
        while (running_.load() && src->running())
            std::this_thread::sleep_for(std::chrono::milliseconds(kLivenessPollMs));
        if (!running_.load())
            return;
        vn::log::warn("videonode-sink: source gone, reconnecting to %s; repeating last frame",
                      p_.socket_path.c_str());
        outage_start_ns_ = last_outage_log_ns_ = mono_ns();
        publish_(nullptr);
    }
}

} // namespace sink
