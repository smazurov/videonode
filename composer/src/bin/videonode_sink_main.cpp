// videonode-sink — single-stream NV12 carrier. Dials a videonode-source SCM_RIGHTS
// socket, mmaps each incoming dma-buf and writes raw NV12 bytes to stdout.
// Used as the producer-side adapter in the single-stream pipeline:
//
//   videonode-source --device /dev/videoN --out-socket /tmp/vn-bus-<id>.sock
//   videonode-sink --socket /tmp/vn-bus-<id>.sock
//     | ffmpeg -f rawvideo -pix_fmt nv12 -s WxH -framerate N -i pipe:0
//              <encoder ...> -f rtsp rtsp://127.0.0.1:8554/<id>
//
// The first frame announces dims on stderr so ffmpeg can be parameterized
// downstream (the daemon already knows the dims it asked the source for,
// so this is purely a sanity log).

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

// emit_frame writes a YUV4MPEG2 frame (NV12 → I420 chroma deinterleave on
// CPU). y4m's per-stream header is written separately on first frame.
bool emit_frame(const scm_rights_source::FrameView& v, std::vector<uint8_t>& uplane,
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

bool emit_y4m_header(int w, int h, int fps_num, int fps_den) {
    char hdr[128];
    int n = std::snprintf(hdr, sizeof(hdr), "YUV4MPEG2 W%d H%d F%d:%d Ip A1:1 C420\n", w, h,
                          fps_num, fps_den);
    return write_full(STDOUT_FILENO,
                      std::span(reinterpret_cast<const uint8_t*>(hdr), static_cast<size_t>(n)));
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
            "videonode-sink — stream NV12 from a videonode-source SCM_RIGHTS socket to stdout.\n"
            "\n"
            "  --socket PATH                Unix socket exposed by videonode-source (required)\n"
            "  -v, --verbose                log per-frame frame_idx/fd to stderr\n"
            "  --poll-ms N                  sleep between consumer polls (default %d)\n"
            "  --settle-ms N                delay after dial before reading (default %d)\n"
            "  --first-frame-timeout N      seconds to wait for first frame (default %d)\n"
            "\n"
            "Stderr announces the NV12 dimensions on first frame so callers know what to\n"
            "feed `ffmpeg -f rawvideo -pix_fmt nv12 -s WxH -framerate N -i pipe:0 ...`.\n",
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
            if (!emit_y4m_header(v.width, v.height, 60, 1))
                break;
            vn::log::info("videonode-sink: streaming Y4M %dx%d (I420 from NV12) from %s",
                          v.width, v.height, a.socket_path.c_str());
            announced = true;
            announced_w = v.width;
            announced_h = v.height;
        }
        if (a.verbose) {
            vn::log::info("videonode-sink: frame_idx=%llu fd=%d",
                          static_cast<unsigned long long>(v.frame_idx), v.fd);
        }
        last_idx = v.frame_idx;
        if (!emit_frame(v, uplane, vplane))
            break;
    }

    src.stop();
    vn::log::info("videonode-sink: shutdown");
    return 0;
}
