// FrameView shape matches FfmpegPipeSource::FrameView so videonode-composer's
// gl_compose code (which already imports NV12 dma-bufs as EGLImages) can
// work against either producer without branching.

#pragma once

#include "src/common/unique_fd.hpp"
#include "src/ipc/dmabuf_header.hpp"

#include <atomic>
#include <cstdint>
#include <mutex>
#include <string>
#include <thread>
#include <vector>

namespace scm_rights_source {

class ScmRightsSource;

// Mirrors ffmpeg_pipe_source::FrameView so the compose layer can substitute one
// for the other.
// Internal snapshot — raw fd ints borrowed from ScmRightsSource internals.
// Not safe to use outside the mutex; kept for the IPC thread.
struct FrameView {
    int fd = -1;
    int width = 0;
    int height = 0;
    uint32_t plane0_pitch = 0;
    uint32_t plane0_offset = 0;
    uint32_t plane1_pitch = 0;
    uint32_t plane1_offset = 0;
    int plane1_fd = -1;
    std::string format;
    dmabuf_header::ColorMatrix color_matrix = dmabuf_header::ColorMatrix::Unspecified;
    uint64_t frame_idx = 0;
    uint64_t slot_index = 0;
    uint64_t generation = 0;
};

// Consumer-facing snapshot with owned (dup'd) fds. Safe to hold across
// frame boundaries — the dma-buf stays alive as long as this struct lives.
struct OwnedFrameView {
    vn::base::unique_fd fd;
    vn::base::unique_fd plane1_fd;
    int width = 0;
    int height = 0;
    uint32_t plane0_pitch = 0;
    uint32_t plane0_offset = 0;
    uint32_t plane1_pitch = 0;
    uint32_t plane1_offset = 0;
    std::string format;
    dmabuf_header::ColorMatrix color_matrix = dmabuf_header::ColorMatrix::Unspecified;
    uint64_t frame_idx = 0;
    uint64_t slot_index = 0;
    uint64_t generation = 0;

    OwnedFrameView() = default;
    OwnedFrameView(OwnedFrameView&& o) noexcept { *this = std::move(o); }
    OwnedFrameView& operator=(OwnedFrameView&& o) noexcept;
    OwnedFrameView(const OwnedFrameView&) = delete;
    OwnedFrameView& operator=(const OwnedFrameView&) = delete;
    ~OwnedFrameView();
};

struct InitParams {
    // Unix socket path. In listen mode (dial=false, default) the source
    // binds + listens on it and accepts the producer's dial. In dial mode
    // (dial=true) the source connects to the path as a client — used when
    // the producer is a long-lived sidecar that listens for N consumers.
    std::string socket_path;
    bool dial = false;
    // Dial mode: keep retrying connect until this elapses. 0 = single
    // attempt, for callers that must not block (canvas render loop).
    int dial_timeout_ms = 30000;
};

class ScmRightsSource {
  public:
    [[nodiscard]] bool init(const InitParams& p);

    // Blocks until the daemon connects (up to ~30 s), then spawns the reader thread.
    [[nodiscard]] bool start();

    // Idempotent.
    void stop();

    ~ScmRightsSource();
    ScmRightsSource() = default;
    ScmRightsSource(const ScmRightsSource&) = delete;
    ScmRightsSource& operator=(const ScmRightsSource&) = delete;

    OwnedFrameView latest_frame() const;

    // Returns an eventfd that becomes readable each time a new frame
    // arrives. Consumers can poll() on this instead of busy-sleeping.
    // Returns -1 if init() hasn't been called yet.
    int notify_fd() const { return notify_fd_.get(); }

    bool running() const { return running_.load(); }
    int listen_fd() const { return listen_fd_.get(); }
    int client_fd() const { return client_fd_.get(); }

  private:
    void thread_main_();

    InitParams params_;
    vn::base::unique_fd listen_fd_;
    vn::base::unique_fd client_fd_;

    std::thread thread_;
    std::atomic<bool> running_{false};
    std::atomic<bool> stop_requested_{false};

    vn::base::unique_fd notify_fd_;

    mutable std::mutex latest_mu_;
    // FrameView's `fd`/`plane1_fd` borrow into latest_owned_fds_. Consumers
    // see raw ints (the view contract); ownership lives in unique_fd here.
    FrameView latest_;
    std::vector<vn::base::unique_fd> latest_owned_fds_;
    // Holds the previous frame's fds so the consumer has a one-cycle window
    // of validity after a new frame replaces latest_.
    std::vector<vn::base::unique_fd> prev_fds_;
};

} // namespace scm_rights_source
