// videonode-sink — SCM_RIGHTS → stdout frame carrier. Dials an SCM_RIGHTS
// socket exposed by either a videonode-source (NV12 capture frames) or a
// videonode-composer with --scm-out (BGRA composed frames), mmaps each
// incoming dma-buf, and writes frames to stdout in a format-appropriate
// wrapping for the downstream ffmpeg consumer.
//
//   videonode-source --device /dev/videoN --out-socket /tmp/vn-bus-<id>.sock
//   videonode-sink --socket /tmp/vn-bus-<id>.sock
//     | ffmpeg -f yuv4mpegpipe -i pipe:0   # NV12 → Y4M auto-detect dims
//              <encoder ...> -f rtsp rtsp://127.0.0.1:8554/<id>
//
//   videonode-composer ... --scm-out /tmp/vn-bus-composer-<id>.sock
//   videonode-sink --socket /tmp/vn-bus-composer-<id>.sock
//     | ffmpeg -f rawvideo -pix_fmt bgra -s WxH -framerate N -i pipe:0
//              <encoder ...> -f rtsp ...
//
// Output format is auto-selected from the first frame's fourcc:
//   - NV*  (NV12, NV24, NV16) → emit YUV4MPEG2 (NV12 only today; chroma
//                                deinterleave to I420 on CPU).
//   - BGRA / ARGB / etc.       → emit raw bytes per frame, no header.
//
// The first frame announces dims + format on stderr so callers can size
// the downstream ffmpeg invocation accordingly.

#include "src/common/log_levels.hpp"
#include "src/ipc/scm_rights_source.hpp"
#include "version.hpp"

#include <chrono>
#include <csignal>
#include <cstdint>
#include <cstdio>
#include <cstdlib>
#include <cstring>
#include <span>
#include <string>
#include <sys/mman.h>
#include <thread>
#include <unistd.h>
#include <vector>

namespace {

volatile std::sig_atomic_t g_running = 1;
void on_sig(int) {
    g_running = 0;
}

// write_full retries until all bytes are flushed or the consumer closes.
bool write_full(int fd, std::span<const uint8_t> buf) {
    while (!buf.empty()) {
        ssize_t w = ::write(fd, buf.data(), buf.size());
        if (w < 0) {
            if (errno == EINTR)
                continue;
            return false;
        }
        if (w == 0)
            return false;
        buf = buf.subspan(static_cast<size_t>(w));
    }
    return true;
}

// emit_frame_nv12_y4m writes a YUV4MPEG2 FRAME (NV12 → I420 chroma
// deinterleave on CPU). y4m stream header is written separately on
// first frame.
bool emit_frame_nv12_y4m(const scm_rights_source::FrameView& v, std::vector<uint8_t>& uplane,
                         std::vector<uint8_t>& vplane) {
    if (v.fd < 0 || v.width <= 0 || v.height <= 0)
        return true;
    size_t y_size = size_t(v.width) * v.height;
    size_t uv_size = y_size / 2;
    void* m = ::mmap(nullptr, y_size + uv_size, PROT_READ, MAP_SHARED, v.fd, 0);
    if (m == MAP_FAILED) {
        vn::log::error("videonode-sink: mmap fd=%d failed: %s", v.fd, strerror(errno));
        return true;
    }
    const auto* y = static_cast<const uint8_t*>(m);
    const auto* uv = y + y_size;
    if (uplane.size() != uv_size / 2)
        uplane.resize(uv_size / 2);
    if (vplane.size() != uv_size / 2)
        vplane.resize(uv_size / 2);
    for (size_t i = 0, j = 0; i < uv_size; i += 2, ++j) {
        uplane[j] = uv[i];
        vplane[j] = uv[i + 1];
    }
    static constexpr uint8_t kFrameTag[] = {'F', 'R', 'A', 'M', 'E', '\n'};
    bool ok = write_full(STDOUT_FILENO, std::span<const uint8_t>(kFrameTag)) &&
              write_full(STDOUT_FILENO, std::span(y, y_size)) &&
              write_full(STDOUT_FILENO, std::span<const uint8_t>(uplane)) &&
              write_full(STDOUT_FILENO, std::span<const uint8_t>(vplane));
    ::munmap(m, y_size + uv_size);
    if (!ok) {
        vn::log::info("videonode-sink: stdout closed, exiting");
        return false;
    }
    return true;
}

// emit_frame_raw_bgra writes one BGRA frame's bytes straight to stdout
// (no per-frame header, no chunking). Downstream ffmpeg consumes via
// `-f rawvideo -pix_fmt bgra -s WxH -framerate N -i pipe:0` — the daemon
// knows the dims it asked the composer to render, so no muxing is needed.
// The composer's canvas BO may have row stride > width*4 on tiled
// backends; pack rows tightly so ffmpeg's -s WxH interpretation matches.
bool emit_frame_raw_bgra(const scm_rights_source::FrameView& v) {
    if (v.fd < 0 || v.width <= 0 || v.height <= 0)
        return true;
    const size_t row_bytes = size_t(v.width) * 4;
    const size_t stride = v.plane0_pitch ? v.plane0_pitch : row_bytes;
    const size_t map_size = v.plane0_offset + stride * v.height;
    void* m = ::mmap(nullptr, map_size, PROT_READ, MAP_SHARED, v.fd, 0);
    if (m == MAP_FAILED) {
        vn::log::error("videonode-sink: mmap fd=%d failed: %s", v.fd, strerror(errno));
        return true;
    }
    const auto* base = static_cast<const uint8_t*>(m) + v.plane0_offset;
    bool ok = true;
    if (stride == row_bytes) {
        ok = write_full(STDOUT_FILENO, std::span(base, row_bytes * v.height));
    } else {
        for (int y = 0; y < v.height && ok; ++y) {
            ok = write_full(STDOUT_FILENO, std::span(base + size_t(y) * stride, row_bytes));
        }
    }
    ::munmap(m, map_size);
    if (!ok) {
        vn::log::info("videonode-sink: stdout closed, exiting");
        return false;
    }
    return true;
}

bool emit_y4m_header(int w, int h, int fps_num, int fps_den) {
    char hdr[128];
    int n = std::snprintf(hdr, sizeof(hdr), "YUV4MPEG2 W%d H%d F%d:%d Ip A1:1 C420\n", w, h,
                          fps_num, fps_den);
    return write_full(STDOUT_FILENO,
                      std::span(reinterpret_cast<const uint8_t*>(hdr), static_cast<size_t>(n)));
}

// is_yuv_nv_format returns true when fourcc names an NV* (NV12/NV24/NV16)
// planar-luma + interleaved-chroma format consumable by the Y4M path.
// Empty fourcc defaults to NV12 for back-compat with legacy senders.
bool is_yuv_nv_format(const std::string& fourcc) {
    return fourcc.empty() || (fourcc.size() == 4 && fourcc[0] == 'N' && fourcc[1] == 'V');
}

} // namespace

namespace {

struct Args {
    std::string socket_path;
    bool verbose = false;
    int poll_ms = 5;     // sleep between latest_frame() polls
    int settle_ms = 200; // delay after dial before first read
    int first_frame_timeout_s = 30;
};

void print_help(const Args& d) {
    fprintf(stderr,
            "videonode-sink — stream frames from an SCM_RIGHTS socket to stdout.\n"
            "\n"
            "  --socket PATH                Unix socket exposed by videonode-source OR\n"
            "                                 a videonode-composer --scm-out (required)\n"
            "  -v, --verbose                log per-frame frame_idx/fd to stderr\n"
            "  --poll-ms N                  sleep between consumer polls (default %d)\n"
            "  --settle-ms N                delay after dial before reading (default %d)\n"
            "  --first-frame-timeout N      seconds to wait for first frame (default %d)\n"
            "\n"
            "Output mode is auto-selected from the first frame's fourcc:\n"
            "  NV12 / NV24 / NV16  → YUV4MPEG2 (NV12-only today; chroma → I420 on CPU).\n"
            "                          Pipe to `ffmpeg -f yuv4mpegpipe -i pipe:0 ...`.\n"
            "  BGRA / ARGB / etc.  → raw bytes per frame (no header).\n"
            "                          Pipe to `ffmpeg -f rawvideo -pix_fmt bgra -s WxH "
            "-framerate N -i pipe:0 ...`.\n"
            "\n"
            "Stderr announces the dims + format on the first frame.\n",
            d.poll_ms, d.settle_ms, d.first_frame_timeout_s);
}

} // namespace

int main(int argc, char** argv) {
    Args a;

    for (int i = 1; i < argc; ++i) {
        std::string s = argv[i];
        auto next = [&](std::string& dst) {
            if (i + 1 < argc)
                dst = argv[++i];
        };
        auto nexti = [&](int& dst) {
            if (i + 1 < argc)
                dst = std::atoi(argv[++i]);
        };
        if (s == "--socket")
            next(a.socket_path);
        else if (s == "-v" || s == "--verbose")
            a.verbose = true;
        else if (s == "--poll-ms")
            nexti(a.poll_ms);
        else if (s == "--settle-ms")
            nexti(a.settle_ms);
        else if (s == "--first-frame-timeout")
            nexti(a.first_frame_timeout_s);
        else if (s == "-h" || s == "--help") {
            print_help(Args{});
            return 0;
        } else if (s == "--version") {
            printf("videonode-sink %s\n", vn::kVersion);
            return 0;
        } else {
            vn::log::error("videonode-sink: unknown arg %s (use --help)", s.c_str());
            return 2;
        }
    }
    if (a.socket_path.empty()) {
        vn::log::error("videonode-sink: --socket PATH is required");
        return 2;
    }

    // stderr defaults to block-buffered when redirected to a file (which
    // the supervisor does via `>log 2>&1`). Force line-buffered so log
    // lines are visible to `tail -f` immediately.
    ::setvbuf(stderr, nullptr, _IOLBF, 0);

    std::signal(SIGINT, on_sig);
    std::signal(SIGTERM, on_sig);
    std::signal(SIGPIPE, SIG_IGN);

    scm_rights_source::ScmRightsSource src;
    scm_rights_source::InitParams p;
    p.socket_path = a.socket_path;
    p.dial = true;
    if (!src.init(p) || !src.start()) {
        vn::log::fatal("videonode-sink: failed to dial %s (is videonode-source up?)",
                       a.socket_path.c_str());
        return 1;
    }
    std::this_thread::sleep_for(std::chrono::milliseconds(a.settle_ms));

    uint64_t last_idx = 0;
    bool announced = false;
    int announced_w = 0, announced_h = 0;
    bool yuv_mode = true; // decided on first frame from FrameView::format
    std::vector<uint8_t> uplane, vplane;
    auto deadline =
        std::chrono::steady_clock::now() + std::chrono::seconds(a.first_frame_timeout_s);
    while (g_running) {
        auto v = src.latest_frame();
        if (v.fd < 0 || v.frame_idx == 0) {
            if (std::chrono::steady_clock::now() > deadline) {
                vn::log::fatal("videonode-sink: timeout waiting for first frame on %s",
                               a.socket_path.c_str());
                src.stop();
                return 1;
            }
            std::this_thread::sleep_for(std::chrono::milliseconds(a.poll_ms));
            continue;
        }
        if (v.frame_idx == last_idx) {
            std::this_thread::sleep_for(std::chrono::milliseconds(a.poll_ms));
            continue;
        }
        if (!announced || v.width != announced_w || v.height != announced_h) {
            if (announced) {
                vn::log::warn("videonode-sink: dimensions changed %dx%d → %dx%d; downstream ffmpeg "
                              "likely needs restart",
                              announced_w, announced_h, v.width, v.height);
            }
            yuv_mode = is_yuv_nv_format(v.format);
            if (yuv_mode) {
                if (!emit_y4m_header(v.width, v.height, 60, 1))
                    break;
                vn::log::info("videonode-sink: streaming Y4M %dx%d (I420 from NV12 fourcc=%s) "
                              "from %s",
                              v.width, v.height, v.format.empty() ? "NV12" : v.format.c_str(),
                              a.socket_path.c_str());
            } else {
                // Raw-bytes mode: no header. Downstream ffmpeg is invoked
                // with -f rawvideo -pix_fmt <format> -s WxH -framerate N.
                vn::log::info("videonode-sink: streaming raw %dx%d (fourcc=%s) from %s",
                              v.width, v.height, v.format.c_str(), a.socket_path.c_str());
            }
            announced = true;
            announced_w = v.width;
            announced_h = v.height;
        }
        if (a.verbose) {
            vn::log::info("videonode-sink: frame_idx=%llu fd=%d",
                          static_cast<unsigned long long>(v.frame_idx), v.fd);
        }
        last_idx = v.frame_idx;
        bool ok = yuv_mode ? emit_frame_nv12_y4m(v, uplane, vplane) : emit_frame_raw_bgra(v);
        if (!ok)
            break;
    }

    src.stop();
    vn::log::info("videonode-sink: shutdown");
    return 0;
}
