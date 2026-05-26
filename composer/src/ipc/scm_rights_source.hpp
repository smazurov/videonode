// scm_rights_source — frame source that receives dma-buf fds from the Go
// daemon over a Unix socket (SCM_RIGHTS), mirroring FfmpegPipeSource's
// public surface so the composer's render loop doesn't care where its
// frames come from.
//
// Lifecycle:
//
//   ScmRightsSource src;
//   src.init({.socket_path = "/tmp/composer-srcA.sock"});
//   src.start();                          // listen + accept + reader thread
//   auto v = src.latest_frame();          // FrameView with current fds
//   src.stop();                           // joins thread, closes fds
//
// The Go daemon's SendDMABuf (internal/composer/dmabuf.go) drives the
// other end. One ScmRightsSource per canvas slot; the daemon connects to
// each socket path the composer is listening on.
//
// FrameView shape matches FfmpegPipeSource::FrameView so videonode-composer's
// gl_compose code (which already imports NV12 dma-bufs as EGLImages) can
// work against either producer without branching.

#pragma once

#include "src/common/unique_fd.hpp"

#include <atomic>
#include <cstdint>
#include <mutex>
#include <string>
#include <thread>
#include <vector>

namespace scm_rights_source {

// FrameView is the snapshot a consumer reads from latest_frame(). Mirrors
// ffmpeg_pipe_source::FrameView so the compose layer can substitute one
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
    uint64_t frame_idx = 0;
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
    uint64_t frame_idx = 0;
};

struct InitParams {
    // Unix socket path. In listen mode (dial=false, default) the source
    // binds + listens on it and accepts the producer's dial. In dial mode
    // (dial=true) the source connects to the path as a client — used when
    // the producer is a long-lived sidecar that listens for N consumers.
    std::string socket_path;
    bool dial = false;
};

class ScmRightsSource {
  public:
    // init binds + listens on InitParams.socket_path. start() then accepts
    // one client and begins the reader thread. Both must run before
    // latest_frame() returns a non-default value.
    [[nodiscard]] bool init(const InitParams& p);

    // start accepts the daemon's connection (blocking until it arrives,
    // up to ~30 s) and spawns the reader thread. Returns true once the
    // thread is running.
    [[nodiscard]] bool start();

    // stop terminates the reader thread, closes the socket and any held
    // dma-buf fds. Idempotent.
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
