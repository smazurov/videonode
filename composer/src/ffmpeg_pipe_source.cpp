#include "ffmpeg_pipe_source.hpp"
#include "child_process.hpp"
#include "dma_heap.hpp" // for sync_start/sync_end on dma-buf fds (works on any dma-buf, not just dma_heap-allocated)

#include <cerrno>
#include <csignal>
#include <cstdio>
#include <cstdlib>
#include <cstring>
#include <sys/mman.h>
#include <unistd.h>

namespace ffmpeg_pipe_source {

namespace {

bool read_exact(int fd, void* dst, size_t n) {
    auto* p = static_cast<uint8_t*>(dst);
    while (n > 0) {
        ssize_t r = ::read(fd, p, n);
        if (r == 0)
            return false; // EOF
        if (r < 0) {
            if (errno == EINTR)
                continue;
            return false;
        }
        p += r;
        n -= r;
    }
    return true;
}

} // namespace

bool FfmpegPipeSource::init(const InitParams& p) {
    params_ = p;
    width_ = p.width;
    height_ = p.height;
    if ((width_ & 1) || (height_ & 1)) {
        fprintf(stderr, "ffmpeg_pipe_source: dims %dx%d must be even (NV12)\n", width_, height_);
        return false;
    }
    if (!p.gbm) {
        fprintf(stderr, "ffmpeg_pipe_source: InitParams.gbm is required (pass EglCtx::gbm())\n");
        return false;
    }
    frame_bytes_ = static_cast<size_t>(width_) * height_ * 3 / 2;

    int ring_n = std::max(2, p.ring_size);
    ring_.resize(ring_n);
    for (int i = 0; i < ring_n; ++i) {
        ring_[i].buf = gbm_alloc::alloc(p.gbm, width_, height_);
        if (!ring_[i].buf.valid()) {
            fprintf(stderr, "ffmpeg_pipe_source: gbm_alloc[%d]\n", i);
            return false;
        }
        // Don't map at init. Per-frame map/unmap is required for radeonsi:
        // a persistent CPU mapping keeps the bo in the CPU-coherent state
        // and subsequent GPU samples read stale/zero. csc-probe's
        // map → write → unmap → sample pattern is the working reference.
    }
    return true;
}

std::vector<std::string> FfmpegPipeSource::build_argv_() const {
    std::vector<std::string> argv = {
        "ffmpeg", "-hide_banner", "-loglevel", "warning", "-nostdin", "-fflags", "nobuffer",
    };

    switch (params_.kind) {
    case ffmpeg_pipe_source::SourceKind::V4L2:
        argv.push_back("-f");
        argv.push_back("v4l2");
        argv.push_back("-input_format");
        argv.push_back(params_.input_format);
        argv.push_back("-video_size");
        argv.push_back(std::to_string(width_) + "x" + std::to_string(height_));
        argv.push_back("-framerate");
        argv.push_back(std::to_string(params_.fps));
        for (const auto& a : params_.extra_input_args)
            argv.push_back(a);
        argv.push_back("-i");
        argv.push_back(params_.device);
        break;
    case ffmpeg_pipe_source::SourceKind::Lavfi:
        // The lavfi expression carries its own size and rate, so we
        // don't pass -video_size / -framerate. Caller is responsible
        // for getting them consistent with width_/height_/fps so
        // downstream NV12 byte-count math stays right.
        argv.push_back("-f");
        argv.push_back("lavfi");
        for (const auto& a : params_.extra_input_args)
            argv.push_back(a);
        argv.push_back("-i");
        argv.push_back(params_.device);
        break;
    }

    argv.push_back("-f");
    argv.push_back("rawvideo");
    argv.push_back("-pix_fmt");
    argv.push_back("nv12");
    argv.push_back("pipe:1");
    return argv;
}

bool FfmpegPipeSource::spawn_ffmpeg_() {
    auto argv = build_argv_();
    auto r = child_process::spawn("ffmpeg", argv, child_process::Direction::StdoutPipe);
    if (r.pid <= 0)
        return false;
    ffmpeg_pid_ = r.pid;
    ffmpeg_stdout_fd_ = r.pipe_fd;

    fprintf(stderr, "ffmpeg_pipe_source: pid=%d cmd:", r.pid);
    for (auto& s : argv)
        fprintf(stderr, " %s", s.c_str());
    fprintf(stderr, "\n");
    return true;
}

bool FfmpegPipeSource::start() {
    if (!spawn_ffmpeg_())
        return false;
    running_.store(true);
    thread_ = std::thread([this] { this->thread_main_(); });
    return true;
}

void FfmpegPipeSource::thread_main_() {
    uint64_t idx = 0;
    int next_slot = 0;
    while (running_.load()) {
        Buf& slot = ring_[next_slot];

        // Map for write, fill, unmap. Per-frame because radeonsi treats
        // the mapping as a CPU-coherent state — only after unmap can the
        // GPU sample the bytes we wrote.
        gbm_alloc::Mapped m = gbm_alloc::map_rw(slot.buf);
        if (!m.y || !m.uv) {
            fprintf(stderr, "ffmpeg_pipe_source: per-frame map failed\n");
            break;
        }
        bool ok = true;
        uint8_t* y_base = static_cast<uint8_t*>(m.y);
        if (slot.buf.y_stride == uint32_t(width_)) {
            ok = read_exact(ffmpeg_stdout_fd_, y_base, size_t(width_) * height_);
        } else {
            for (int y = 0; y < height_ && ok; ++y)
                ok = read_exact(ffmpeg_stdout_fd_, y_base + y * slot.buf.y_stride, width_);
        }
        if (ok) {
            uint8_t* uv_base = static_cast<uint8_t*>(m.uv);
            if (slot.buf.uv_stride == uint32_t(width_)) {
                ok = read_exact(ffmpeg_stdout_fd_, uv_base, size_t(width_) * (height_ / 2));
            } else {
                for (int y = 0; y < height_ / 2 && ok; ++y)
                    ok = read_exact(ffmpeg_stdout_fd_, uv_base + y * slot.buf.uv_stride, width_);
            }
        }
        gbm_alloc::unmap(slot.buf);
        if (!ok) {
            if (running_.load()) {
                fprintf(stderr, "ffmpeg_pipe_source: read_exact EOF/err; ffmpeg died?\n");
            }
            break;
        }

        ++idx;
        {
            std::lock_guard<std::mutex> g(latest_mu_);
            latest_.fd = slot.buf.y_fd;
            latest_.plane1_fd = slot.buf.uv_fd;
            latest_.width = width_;
            latest_.height = height_;
            latest_.plane0_pitch = slot.buf.y_stride;
            latest_.plane0_offset = 0;
            latest_.plane1_pitch = slot.buf.uv_stride;
            latest_.plane1_offset = 0;
            latest_.frame_idx = idx;
        }
        next_slot = (next_slot + 1) % static_cast<int>(ring_.size());
    }
    running_.store(false);
}

void FfmpegPipeSource::stop() {
    running_.store(false);
    // Closing the pipe first nudges ffmpeg to exit; reap() then waits +
    // escalates to SIGTERM/SIGKILL if needed.
    int pipe_fd = ffmpeg_stdout_fd_;
    pid_t pid = ffmpeg_pid_;
    ffmpeg_stdout_fd_ = -1;
    ffmpeg_pid_ = -1;
    child_process::reap(pid, pipe_fd, /*graceful_ms=*/5000);
    if (thread_.joinable())
        thread_.join();
}

FfmpegPipeSource::~FfmpegPipeSource() {
    stop();
    for (auto& s : ring_) {
        s.mapped_y = s.mapped_uv = nullptr;
        gbm_alloc::free(s.buf);
    }
}

FrameView FfmpegPipeSource::latest_frame() const {
    std::lock_guard<std::mutex> g(latest_mu_);
    return latest_;
}

} // namespace ffmpeg_pipe_source
