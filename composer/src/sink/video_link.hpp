#pragma once

#include "src/ipc/scm_rights_source.hpp"

#include <atomic>
#include <cstdint>
#include <memory>
#include <mutex>
#include <string>
#include <thread>

namespace sink {

[[nodiscard]] scm_rights_source::FrameView view_of(const scm_rights_source::OwnedFrameView& f);

// Keeps an SCM_RIGHTS connection to the producer on a background thread, so a
// producer restart never blocks the output loop.
class VideoLink {
  public:
    struct Params {
        std::string socket_path;
        int settle_ms = 200;
        int initial_dial_timeout_ms = 30000;
    };

    explicit VideoLink(Params p);
    ~VideoLink();
    VideoLink(const VideoLink&) = delete;
    VideoLink& operator=(const VideoLink&) = delete;

    void start();
    void stop();

    [[nodiscard]] std::shared_ptr<scm_rights_source::ScmRightsSource> current() const;
    [[nodiscard]] uint64_t generation() const { return generation_.load(); }
    [[nodiscard]] bool initial_dial_failed() const { return initial_dial_failed_.load(); }

  private:
    void thread_main_();
    [[nodiscard]] std::shared_ptr<scm_rights_source::ScmRightsSource> dial_();
    void publish_(std::shared_ptr<scm_rights_source::ScmRightsSource> src);
    void note_still_down_(int64_t now);
    void note_connected_(int64_t now);

    Params p_;
    std::thread thread_;
    std::atomic<bool> running_{false};
    std::atomic<bool> initial_dial_failed_{false};
    std::atomic<uint64_t> generation_{0};
    mutable std::mutex mu_;
    std::shared_ptr<scm_rights_source::ScmRightsSource> current_;
    int failed_dials_ = 0;
    int64_t outage_start_ns_ = -1;
    int64_t last_outage_log_ns_ = -1;
};

} // namespace sink
