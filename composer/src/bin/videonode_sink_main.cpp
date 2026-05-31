// videonode-sink — SCM_RIGHTS → stdout frame carrier. Dials an SCM_RIGHTS
// socket exposed by a videonode-source or a videonode-composer with
// --scm-out (both broadcast NV12 dma-bufs), mmaps each incoming dma-buf,
// and writes raw NV12 bytes (Y plane, then UV plane) to stdout for the
// downstream ffmpeg consumer.
//
//   videonode-source --device /dev/videoN --out-socket /tmp/vn-bus-<id>.sock
//   videonode-sink --socket /tmp/vn-bus-<id>.sock
//     | ffmpeg -f rawvideo -pix_fmt nv12 -video_size WxH -framerate N
//              -i pipe:0 <encoder ...> -f rtsp rtsp://127.0.0.1:8554/<id>
//
// The first frame announces dims on stderr so callers can size the
// downstream ffmpeg invocation accordingly.

#include "src/common/log_levels.hpp"
#include "src/common/raise_fd_limit.hpp"
#include "src/common/signal.hpp"
#include "src/ipc/scm_rights_source.hpp"
#include "version.hpp"

#include <atomic>
#include <chrono>
#include <csignal>
#include <cstdint>
#include <cstdio>
#include <cstdlib>
#include <cstring>
#include <linux/dma-buf.h>
#include <memory>
#include <poll.h>
#include <span>
#include <string>
#include <sys/ioctl.h>
#include <sys/mman.h>
#include <thread>
#include <unistd.h>

#ifdef __SSE4_1__
#include <immintrin.h>
#endif

namespace {

std::atomic<bool> g_running{true};

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

void dmabuf_sync_start(int fd) {
    struct dma_buf_sync sync{};
    sync.flags = DMA_BUF_SYNC_START | DMA_BUF_SYNC_READ;
    ::ioctl(fd, DMA_BUF_IOCTL_SYNC, &sync);
}

void dmabuf_sync_end(int fd) {
    struct dma_buf_sync sync{};
    sync.flags = DMA_BUF_SYNC_END | DMA_BUF_SYNC_READ;
    ::ioctl(fd, DMA_BUF_IOCTL_SYNC, &sync);
}

// emit_frame_nv12_raw mmaps the producer's NV12 dma-buf(s) and writes
// raw NV12 bytes (Y plane then UV plane) to stdout. No format conversion —
// ffmpeg reads via `-f rawvideo -pix_fmt nv12 -video_size WxH -framerate N`.
//
// Honors stride padding: rows are packed tightly in the output even when
// the allocator pads them (e.g. GBM tiled BOs).
// Reusable scratch buffer — avoids per-frame allocation. Aligned to 16
// bytes for SSE streaming stores.
static std::vector<uint8_t, std::allocator<uint8_t>> g_scratch;
static uint8_t* aligned_scratch(size_t needed) {
    if (g_scratch.size() < needed + 15)
        g_scratch.resize(needed + 15);
    auto p = reinterpret_cast<uintptr_t>(g_scratch.data());
    return reinterpret_cast<uint8_t*>((p + 15) & ~uintptr_t(15));
}

// streaming_memcpy uses non-temporal loads (movntdqa) to read from
// write-combining memory efficiently, then regular stores into cached dst.
// Unrolled 4x streaming load for reading write-combining memory.
// movntdqa bypasses the WC serialization bottleneck that makes regular
// loads ~100x slower than cached reads on GPU-allocated dma-bufs.
void streaming_memcpy(uint8_t* dst, const uint8_t* src, size_t len) {
#ifdef __SSE4_1__
    size_t n = len / 16;
    std::span<uint8_t> dst_bytes(dst, len);
    std::span<const uint8_t> src_bytes(src, len);
    std::span<__m128i> dst_blocks(reinterpret_cast<__m128i*>(dst_bytes.data()), n);
    std::span<__m128i> src_blocks(
        const_cast<__m128i*>(reinterpret_cast<const __m128i*>(src_bytes.data())), n);
    size_t i = 0;
    for (; i + 4 <= n; i += 4) {
        __m128i r0 = _mm_stream_load_si128(&src_blocks[i + 0]);
        __m128i r1 = _mm_stream_load_si128(&src_blocks[i + 1]);
        __m128i r2 = _mm_stream_load_si128(&src_blocks[i + 2]);
        __m128i r3 = _mm_stream_load_si128(&src_blocks[i + 3]);
        _mm_store_si128(&dst_blocks[i + 0], r0);
        _mm_store_si128(&dst_blocks[i + 1], r1);
        _mm_store_si128(&dst_blocks[i + 2], r2);
        _mm_store_si128(&dst_blocks[i + 3], r3);
    }
    for (; i < n; ++i) {
        _mm_store_si128(&dst_blocks[i], _mm_stream_load_si128(&src_blocks[i]));
    }
    size_t tail = len & 15;
    if (tail > 0)
        std::memcpy(dst_bytes.subspan(n * 16).data(), src_bytes.subspan(n * 16).data(), tail);
    _mm_sfence();
#else
    std::memcpy(dst, src, len);
#endif
}

void copy_plane(uint8_t* dst, const uint8_t* src, size_t width, size_t pitch, size_t rows) {
    if (pitch == width) {
        streaming_memcpy(dst, src, width * rows);
    } else {
        std::span<uint8_t> dst_span(dst, width * rows);
        std::span<const uint8_t> src_span(src, pitch * rows);
        for (size_t r = 0; r < rows; ++r)
            streaming_memcpy(dst_span.subspan(r * width, width).data(),
                             src_span.subspan(r * pitch, width).data(), width);
    }
}

bool emit_frame_nv12_raw(const scm_rights_source::FrameView& v) {
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
    const size_t uv_raw_bytes = uv_pitch * uv_rows;
    const size_t frame_bytes = width * height + width * uv_rows;

    uint8_t* scratch = aligned_scratch(frame_bytes);

    const size_t y_map_size = v.plane0_offset + y_pitch * height +
                              (v.plane1_fd >= 0 ? 0 : uv_raw_bytes + v.plane1_offset);
    void* y_map = ::mmap(nullptr, y_map_size, PROT_READ, MAP_SHARED, v.fd, 0);
    if (y_map == MAP_FAILED) {
        vn::log::error("videonode-sink: mmap y_fd=%d failed: %s", v.fd, strerror(errno));
        return true;
    }
    dmabuf_sync_start(v.fd);
    std::span<const uint8_t> y_map_span(static_cast<const uint8_t*>(y_map), y_map_size);
    const auto* y_base = y_map_span.subspan(v.plane0_offset).data();

    void* uv_map = nullptr;
    size_t uv_map_size = 0;
    const uint8_t* uv_base = nullptr;
    if (v.plane1_fd >= 0) {
        uv_map_size = v.plane1_offset + uv_raw_bytes;
        uv_map = ::mmap(nullptr, uv_map_size, PROT_READ, MAP_SHARED, v.plane1_fd, 0);
        if (uv_map == MAP_FAILED) {
            vn::log::error("videonode-sink: mmap uv_fd=%d failed: %s", v.plane1_fd,
                           strerror(errno));
            dmabuf_sync_end(v.fd);
            ::munmap(y_map, y_map_size);
            return true;
        }
        dmabuf_sync_start(v.plane1_fd);
        std::span<const uint8_t> uv_map_span(static_cast<const uint8_t*>(uv_map), uv_map_size);
        uv_base = uv_map_span.subspan(v.plane1_offset).data();
    } else {
        size_t uv_off = v.plane1_offset ? v.plane1_offset : v.plane0_offset + y_pitch * height;
        uv_base = y_map_span.subspan(uv_off).data();
    }

    // Copy from (potentially uncached) dma-buf into the cached scratch
    // buffer in one pass, then release the mmap before writing to the pipe.
    std::span<uint8_t> scratch_span(scratch, frame_bytes);
    copy_plane(scratch_span.subspan(0, width * height).data(), y_base, width, y_pitch, height);
    copy_plane(scratch_span.subspan(width * height).data(), uv_base, width, uv_pitch, uv_rows);

    if (v.plane1_fd >= 0 && uv_map != nullptr) {
        dmabuf_sync_end(v.plane1_fd);
        ::munmap(uv_map, uv_map_size);
    }
    dmabuf_sync_end(v.fd);
    ::munmap(y_map, y_map_size);

    bool ok = write_full(STDOUT_FILENO, std::span(scratch, frame_bytes));
    if (!ok) {
        vn::log::info("videonode-sink: stdout closed, exiting");
        return false;
    }
    return true;
}

} // namespace

namespace {

struct Args {
    std::string socket_path;
    bool verbose = false;
    int poll_ms = 5;
    int settle_ms = 200;
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
            "Emits raw NV12 bytes (Y plane, then UV plane) per frame.\n"
            "  Pipe to `ffmpeg -f rawvideo -pix_fmt nv12 -video_size WxH\n"
            "           -framerate N -i pipe:0 ...`.\n"
            "\n"
            "Stderr announces the dims on the first frame.\n",
            d.poll_ms, d.settle_ms, d.first_frame_timeout_s);
}

// parse_args returns -1 on parse error (caller should return 2), 0 on success,
// or 1 if an early-exit flag (--help/--version) was handled (caller returns 0).
int parse_args(int argc, char** argv, Args& a) {
    std::span<char*> args(argv, static_cast<size_t>(argc));
    for (int i = 1; i < argc; ++i) {
        std::string s = args[static_cast<size_t>(i)];
        auto next = [&](std::string& dst) {
            if (i + 1 < argc)
                dst = args[static_cast<size_t>(++i)];
        };
        auto nexti = [&](int& dst) {
            if (i + 1 < argc)
                dst = std::atoi(args[static_cast<size_t>(++i)]);
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
            return 1;
        } else if (s == "--version") {
            printf("videonode-sink %s\n", vn::kVersion);
            return 1;
        } else {
            vn::log::error("videonode-sink: unknown arg %s (use --help)", s.c_str());
            return -1;
        }
    }
    if (a.socket_path.empty()) {
        vn::log::error("videonode-sink: --socket PATH is required");
        return -1;
    }
    return 0;
}

// Outcome of run_frame_loop, telling main() whether to reconnect or exit.
enum class LoopExit {
    SourceDropped,     // reader thread died (reset/peer-closed) — re-dial
    StdoutClosed,      // downstream ffmpeg gone — exit
    FormatChanged,     // dims/format differ from announced — exit for rebuild
    FirstFrameTimeout, // connected but no first frame within the deadline
    Shutdown,          // signal-requested shutdown
};

// Format/dims announced on the first frame, persisted across reconnects so a
// same-format source restart resumes the existing ffmpeg pipeline untouched
// while a genuine reconfiguration forces a clean exit.
struct StreamShape {
    bool announced = false;
    int width = 0;
    int height = 0;
};

void wait_for_frame(int notify_fd, int timeout_ms) {
    if (notify_fd >= 0) {
        pollfd pfd{.fd = notify_fd, .events = POLLIN, .revents = 0};
        (void)::poll(&pfd, 1, timeout_ms);
        if (pfd.revents & POLLIN) {
            uint64_t val = 0;
            (void)::read(notify_fd, &val, sizeof(val));
        }
    } else {
        std::this_thread::sleep_for(std::chrono::milliseconds(timeout_ms));
    }
}

[[nodiscard]] LoopExit run_frame_loop(scm_rights_source::ScmRightsSource& src, const Args& a,
                                      StreamShape& shape) {
    uint64_t last_idx = 0;
    int nfd = src.notify_fd();
    auto deadline =
        std::chrono::steady_clock::now() + std::chrono::seconds(a.first_frame_timeout_s);
    while (g_running.load()) {
        if (!src.running())
            return LoopExit::SourceDropped;
        auto owned = src.latest_frame();
        if (owned.fd.get() < 0 || owned.frame_idx == 0) {
            if (std::chrono::steady_clock::now() > deadline)
                return LoopExit::FirstFrameTimeout;
            wait_for_frame(nfd, a.poll_ms);
            continue;
        }
        if (owned.frame_idx == last_idx) {
            wait_for_frame(nfd, a.poll_ms);
            continue;
        }
        if (shape.announced && (owned.width != shape.width || owned.height != shape.height)) {
            vn::log::warn("videonode-sink: source reconfigured %dx%d -> %dx%d; exiting so "
                          "the encoder rebuilds",
                          shape.width, shape.height, owned.width, owned.height);
            return LoopExit::FormatChanged;
        }
        if (!shape.announced) {
            shape.width = owned.width;
            shape.height = owned.height;
            shape.announced = true;
            vn::log::info("videonode-sink: streaming raw NV12 %dx%d from %s", owned.width,
                          owned.height, a.socket_path.c_str());
        }
        if (a.verbose) {
            vn::log::info("videonode-sink: frame_idx=%llu fd=%d",
                          static_cast<unsigned long long>(owned.frame_idx), owned.fd.get());
        }
        last_idx = owned.frame_idx;
        scm_rights_source::FrameView v;
        v.fd = owned.fd.get();
        v.plane1_fd = owned.plane1_fd.get();
        v.width = owned.width;
        v.height = owned.height;
        v.plane0_pitch = owned.plane0_pitch;
        v.plane0_offset = owned.plane0_offset;
        v.plane1_pitch = owned.plane1_pitch;
        v.plane1_offset = owned.plane1_offset;
        v.format = owned.format;
        v.frame_idx = owned.frame_idx;
        bool ok = emit_frame_nv12_raw(v);
        if (!ok)
            return LoopExit::StdoutClosed;
    }
    return LoopExit::Shutdown;
}

// dial_source dials the producer socket (start() retries the dial ~30s with
// backoff). Returns a started source, or nullptr on failure — the caller
// decides whether that is fatal (initial connect) or a transient re-dial miss.
[[nodiscard]] std::unique_ptr<scm_rights_source::ScmRightsSource> dial_source(const Args& a) {
    auto src = std::make_unique<scm_rights_source::ScmRightsSource>();
    scm_rights_source::InitParams p;
    p.socket_path = a.socket_path;
    p.dial = true;
    if (!src->init(p) || !src->start())
        return nullptr;
    return src;
}

} // namespace

int main(int argc, char** argv) {
    vn::raise_fd_limit();

    Args a;
    int parse_result = parse_args(argc, argv, a);
    if (parse_result != 0)
        return parse_result > 0 ? 0 : 2;

    ::setvbuf(stderr, nullptr, _IOLBF, 0);
    vn::signal::install_shutdown(g_running);

    // Reconnect loop: a videonode-source/composer restart drops our SCM
    // connection mid-stream. Re-dial and resume feeding the same downstream
    // ffmpeg rather than exiting and forcing an encoder teardown + WebRTC
    // renegotiation. `shape` persists across dials so a same-format restart is
    // seamless; a genuine reconfiguration or a closed stdout exits instead.
    StreamShape shape;
    int rc = 0;
    while (g_running.load()) {
        auto src = dial_source(a);
        if (!src) {
            if (!shape.announced) {
                vn::log::fatal("videonode-sink: failed to dial %s (is videonode-source up?)",
                               a.socket_path.c_str());
                rc = 1;
                break;
            }
            vn::log::warn("videonode-sink: re-dial %s failed, retrying", a.socket_path.c_str());
            continue;
        }
        std::this_thread::sleep_for(std::chrono::milliseconds(a.settle_ms));

        LoopExit reason = run_frame_loop(*src, a, shape);
        src->stop();

        if (reason == LoopExit::FirstFrameTimeout && !shape.announced) {
            vn::log::fatal("videonode-sink: timeout waiting for first frame on %s",
                           a.socket_path.c_str());
            rc = 1;
            break;
        }
        if (reason == LoopExit::SourceDropped || reason == LoopExit::FirstFrameTimeout) {
            if (!g_running.load())
                break;
            vn::log::info("videonode-sink: source gone, reconnecting to %s", a.socket_path.c_str());
            continue;
        }
        if (reason == LoopExit::StdoutClosed)
            vn::log::info("videonode-sink: stdout closed, exiting");
        break; // StdoutClosed / FormatChanged / Shutdown
    }
    vn::log::info("videonode-sink: shutdown");
    return rc;
}
