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
//                                deinterleave to I420 on CPU via Y4mWriter).
//   - BGRA / ARGB / etc.       → emit raw bytes per frame, no header.
//
// The first frame announces dims + format on stderr so callers can size
// the downstream ffmpeg invocation accordingly.

#include "src/common/log_levels.hpp"
#include "src/common/signal.hpp"
#include "src/ipc/scm_rights_source.hpp"
#include "src/process/y4m_writer.hpp"
#include "version.hpp"

#include <atomic>
#include <chrono>
#include <csignal>
#include <cstdint>
#include <cstdio>
#include <cstdlib>
#include <cstring>
#include <memory>
#include <span>
#include <string>
#include <sys/mman.h>
#include <thread>
#include <unistd.h>
#include <vector>

namespace {

std::atomic<bool> g_running{true};

// write_full retries until all bytes are flushed or the consumer closes.
// Used for the raw-BGRA path; the Y4M path lives in vn::process::Y4mWriter.
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

// emit_frame_nv12_y4m mmaps the producer's NV12 dma-buf(s) and hands plane
// spans to the Y4mWriter for a YUV4MPEG2 FRAME.
//
// Honors the source's reported strides + plane layout:
//   - plane0_pitch / plane1_pitch may be > width when the allocator
//     pads rows (e.g. GBM tiled BOs); Y4mWriter packs rows tightly.
//   - plane1_fd >= 0 → separate dma-buf for the UV plane; mmap each
//     side independently. Otherwise UV lives in the same fd at
//     plane1_offset (or contiguous after Y when plane1_offset==0).
bool emit_frame_nv12_y4m(const scm_rights_source::FrameView& v, vn::process::Y4mWriter& writer) {
    if (v.fd < 0 || v.width <= 0 || v.height <= 0)
        return true;
    const size_t width = size_t(v.width);
    const size_t height = size_t(v.height);
    const size_t y_pitch = v.plane0_pitch ? v.plane0_pitch : width;
    const size_t uv_pitch = v.plane1_pitch ? v.plane1_pitch : width;
    if (y_pitch < width || uv_pitch < width) {
        vn::log::error("videonode-sink: NV12 stride < width (y=%zu uv=%zu w=%zu); dropping frame",
                       y_pitch, uv_pitch, width);
        return true;
    }
    const size_t uv_rows = height / 2;
    const size_t uv_raw_bytes = uv_pitch * uv_rows; // mmap'd region size

    // Map Y plane (possibly with the UV plane appended in single-fd mode).
    const size_t y_map_size = v.plane0_offset + y_pitch * height +
                              (v.plane1_fd >= 0 ? 0 : uv_raw_bytes + v.plane1_offset);
    void* y_map = ::mmap(nullptr, y_map_size, PROT_READ, MAP_SHARED, v.fd, 0);
    if (y_map == MAP_FAILED) {
        vn::log::error("videonode-sink: mmap y_fd=%d failed: %s", v.fd, strerror(errno));
        return true;
    }
    const auto* y_base = static_cast<const uint8_t*>(y_map) + v.plane0_offset;

    // Map UV plane: separate fd, or same fd at plane1_offset.
    void* uv_map = nullptr;
    size_t uv_map_size = 0;
    const uint8_t* uv_base = nullptr;
    if (v.plane1_fd >= 0) {
        uv_map_size = v.plane1_offset + uv_raw_bytes;
        uv_map = ::mmap(nullptr, uv_map_size, PROT_READ, MAP_SHARED, v.plane1_fd, 0);
        if (uv_map == MAP_FAILED) {
            vn::log::error("videonode-sink: mmap uv_fd=%d failed: %s", v.plane1_fd,
                           strerror(errno));
            ::munmap(y_map, y_map_size);
            return true;
        }
        uv_base = static_cast<const uint8_t*>(uv_map) + v.plane1_offset;
    } else {
        // Single-fd NV12: UV starts at v.plane1_offset, or right after Y
        // when plane1_offset is the default zero.
        size_t uv_off = v.plane1_offset ? v.plane1_offset : v.plane0_offset + y_pitch * height;
        uv_base = static_cast<const uint8_t*>(y_map) + uv_off;
    }

    bool ok = writer.WriteFrameNV12(std::span<const uint8_t>(y_base, y_pitch * height), y_pitch,
                                    std::span<const uint8_t>(uv_base, uv_raw_bytes), uv_pitch);
    if (uv_map != nullptr) {
        ::munmap(uv_map, uv_map_size);
    }
    ::munmap(y_map, y_map_size);
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
    // Reject obviously-malformed pitches — a producer reporting
    // stride < row_bytes would cause our row loop to read past each
    // row boundary into the next row's prefix, producing sheared
    // output. Treat as a recoverable frame-drop (return true =
    // continue with next frame) rather than exit.
    if (stride < row_bytes) {
        vn::log::error("videonode-sink: BGRA stride %zu < row_bytes %zu (width=%d); dropping frame",
                       stride, row_bytes, v.width);
        return true;
    }
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

// is_nv12_format returns true when fourcc names NV12 specifically.
// Empty fourcc defaults to NV12 for back-compat with legacy senders.
// NV16 (4:2:2) and NV24 (4:4:4) deliberately fall through to raw mode
// even though their fourccs start with "NV" — the Y4M path's chroma
// layout is hardcoded to 4:2:0 and would under-map and mis-classify
// the chroma plane otherwise. Future work: a real NV16/NV24 Y4M path
// (CHROMA422 / CHROMA444 in the YUV4MPEG2 header).
bool is_nv12_format(const std::string& fourcc) {
    return fourcc.empty() || fourcc == "NV12";
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

    vn::signal::install_shutdown(g_running);

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
    std::unique_ptr<vn::process::Y4mWriter> y4m;
    auto deadline =
        std::chrono::steady_clock::now() + std::chrono::seconds(a.first_frame_timeout_s);
    while (g_running.load()) {
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
            yuv_mode = is_nv12_format(v.format);
            if (yuv_mode) {
                y4m = std::make_unique<vn::process::Y4mWriter>(STDOUT_FILENO, v.width, v.height, 60,
                                                               1);
                if (!y4m->WriteHeader())
                    break;
                vn::log::info("videonode-sink: streaming Y4M %dx%d (I420 from NV12 fourcc=%s) "
                              "from %s",
                              v.width, v.height, v.format.empty() ? "NV12" : v.format.c_str(),
                              a.socket_path.c_str());
            } else {
                // Raw-bytes mode: no header. Downstream ffmpeg is invoked
                // with -f rawvideo -pix_fmt <format> -s WxH -framerate N.
                vn::log::info("videonode-sink: streaming raw %dx%d (fourcc=%s) from %s", v.width,
                              v.height, v.format.c_str(), a.socket_path.c_str());
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
        bool ok = yuv_mode ? emit_frame_nv12_y4m(v, *y4m) : emit_frame_raw_bgra(v);
        if (!ok)
            break;
    }

    src.stop();
    vn::log::info("videonode-sink: shutdown");
    return 0;
}
