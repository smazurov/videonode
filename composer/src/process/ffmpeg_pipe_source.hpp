// ffmpeg_pipe_source — child-ffmpeg-process NV12 frame producer.
//
// What this does: spawns an `ffmpeg` subprocess configured to capture from a
// V4L2 device (HDMI-IN, UVC, anything ffmpeg understands), decode whatever
// format the device delivers (NV12, MJPEG, YUYV, ...), and emit raw NV12
// frames on its stdout. We read those frames in a dedicated capture thread
// and copy each one into a dma-heap NV12 buffer that GLES can sample as a
// dma-buf EGLImage.
//
// Why an ffmpeg subprocess for capture:
//   - HDMI-IN on rk_hdmirx uses multiplanar V4L2 ioctls (different from UVC).
//     ffmpeg's v4l2 demuxer handles both planar layouts and format-change
//     events; we get that for free.
//   - Lyra UVC delivers only MJPEG at 4K/1080p. ffmpeg uses MPP MJPEG decode
//     internally (the ffmpeg-rockchip build sees mjpeg_rkmpp and rkrga is
//     compiled in), giving us hardware-accelerated decode without writing
//     an MPP wrapper here. (mpp_jpeg_dec.hpp exists in the tree for when we
//     do want to bypass ffmpeg for tighter latency control.)
//   - The capture side becomes a one-line command string instead of a few
//     hundred lines of ioctl plumbing. This source validates the GPU compose
//     path, not the V4L2 plumbing path.
//
// Cost: one memcpy per frame from pipe -> dma-heap buffer. At 1080p30 that's
// ~93 MB/s of CPU bandwidth per source; at 4K30 ~373 MB/s. Tolerable for the
// host-dev path. For production we'd swap this for direct V4L2 EXPBUF +
// (optional) in-process MPP decode.
//
// Threading: each source owns one capture thread that DQBUFs from the pipe
// and ping-pongs between two dma-heap buffers. Public methods are
// thread-safe; consumers call latest_frame() to get the most recent fully-
// written buffer's fd.

#pragma once

#include "src/render/gbm_alloc.hpp"

#include <atomic>
#include <cstdint>
#include <mutex>
#include <span>
#include <string>
#include <sys/types.h>
#include <thread>
#include <vector>

namespace ffmpeg_pipe_source {

// Source backend selector. V4L2 is the production path; Lavfi lets us
// produce a synthetic input on machines that don't have a usable capture
// device (e.g. the dev machine — no HDMI-IN, maybe no UVC camera handy).
enum class SourceKind {
    V4L2,
    Lavfi,
};

struct InitParams {
    SourceKind kind = SourceKind::V4L2;
    std::string device;       // /dev/videoN, or lavfi expression like
                              // "testsrc2=size=1920x1080:rate=30"
    std::string input_format; // "nv12" / "mjpeg" / "yuyv422"; unused for Lavfi
    int width = 1920;
    int height = 1080;
    int fps = 60;
    int ring_size = 3; // number of GBM buffers; 3 lets one consumer
                       //   hold a frame while capture writes the next
                       //   without blocking the third for the kernel.
    // GBM device used to allocate the frame ring. Required. Mesa's GBM
    // gives us NV12-compatible dma-bufs that radeonsi / panthor / anv
    // all accept on import — dma_heap-backed NV12 fails on radeonsi.
    struct gbm_device* gbm = nullptr;

    // If non-empty, extra args spliced in front of the input URL. Useful for
    // raw-bytes input variants we might want later (e.g. "-thread_queue_size 1024").
    std::vector<std::string> extra_input_args;
};

struct FrameView {
    int fd = -1; // dma-buf fd (caller borrows; lifetime = source)
    int width = 0;
    int height = 0;
    uint32_t plane0_pitch = 0;
    uint32_t plane0_offset = 0;
    uint32_t plane1_pitch = 0;
    uint32_t plane1_offset = 0;
    // -1 = single-fd NV12 (chroma packed in the same dma-buf, which is what
    // ffmpeg writes today). Field present so this view stays structurally
    // identical to scm_rights_source::FrameView; main.cpp templates over both.
    int plane1_fd = -1;
    // DRM fourcc string — ffmpeg child always outputs NV12 today, so this
    // is a constant. Present so the canonical FrameView in main.cpp can
    // template over both source types without branching.
    std::string format = "NV12";
    uint64_t frame_idx = 0; // monotonically increasing; 0 means "no frame yet"
};

class FfmpegPipeSource {
  public:
    [[nodiscard]] bool init(const InitParams& p);
    [[nodiscard]] bool start();
    void stop();
    ~FfmpegPipeSource();

    FfmpegPipeSource() = default;
    FfmpegPipeSource(const FfmpegPipeSource&) = delete;
    FfmpegPipeSource& operator=(const FfmpegPipeSource&) = delete;

    FrameView latest_frame() const;

    int width() const { return width_; }
    int height() const { return height_; }
    bool running() const { return running_.load(); }
    pid_t pid() const { return ffmpeg_pid_; }

  private:
    bool spawn_ffmpeg_();
    std::vector<std::string> build_argv_() const;
    void thread_main_();
    bool read_full_(std::span<uint8_t> dst);

    InitParams params_;
    int width_ = 0;
    int height_ = 0;
    size_t frame_bytes_ = 0; // width*height*3/2 for NV12

    pid_t ffmpeg_pid_ = -1;
    int ffmpeg_stdout_fd_ = -1;

    struct Buf {
        gbm_alloc::Nv12Buf buf;
        void* mapped_y = nullptr;
        void* mapped_uv = nullptr;
    };
    std::vector<Buf> ring_;

    std::thread thread_;
    std::atomic<bool> running_{false};

    mutable std::mutex latest_mu_;
    FrameView latest_;
};

} // namespace ffmpeg_pipe_source
