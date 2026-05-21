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
// FrameView shape matches FfmpegPipeSource::FrameView so composer-spike's
// gl_compose code (which already imports NV12 dma-bufs as EGLImages) can
// work against either producer without branching.

#pragma once

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
struct FrameView {
    // Primary plane fd — owned by ScmRightsSource. Consumers must finish
    // using it before the next incoming message replaces the slot (Go
    // sender's cadence). For tighter sync the consumer should dup().
    int fd = -1;

    int width = 0;
    int height = 0;
    uint32_t plane0_pitch = 0;
    uint32_t plane0_offset = 0;
    uint32_t plane1_pitch = 0; // 0 when single-plane
    uint32_t plane1_offset = 0;
    int plane1_fd = -1; // -1 when single-plane; else second fd

    // DRM fourcc string as carried over the wire ("NV12" / "NV24" / "NV16"
    // / "BG24" for BGR888). main.cpp maps this to EGL_LINUX_DRM_FOURCC_EXT.
    // Empty string defaults to NV12 for back-compat with senders that
    // don't fill Format.
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

    // Thread-safe snapshot.
    FrameView latest_frame() const;

    bool running() const { return running_.load(); }
    int listen_fd() const { return listen_fd_; }
    int client_fd() const { return client_fd_; }

  private:
    void thread_main_();

    InitParams params_;
    int listen_fd_ = -1;
    int client_fd_ = -1;

    std::thread thread_;
    std::atomic<bool> running_{false};
    std::atomic<bool> stop_requested_{false};

    mutable std::mutex latest_mu_;
    FrameView latest_;
    // Holds the previous frame's fds so we can close them on the NEXT
    // frame arrival (gives the consumer some window of validity).
    std::vector<int> prev_fds_;
};

} // namespace scm_rights_source
