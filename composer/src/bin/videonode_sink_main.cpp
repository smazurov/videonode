// videonode-sink — SCM_RIGHTS → stdout frame carrier. Dials an SCM_RIGHTS
// socket exposed by either a videonode-source (NV12 capture frames) or a
// videonode-composer with --scm-out (BGRA composed frames), mmaps each
// incoming dma-buf, and writes frames to stdout for the downstream ffmpeg
// consumer.
//
//   videonode-source --device /dev/videoN --out-socket /tmp/vn-bus-<id>.sock
//   videonode-sink --socket /tmp/vn-bus-<id>.sock
//     | ffmpeg -f rawvideo -pix_fmt nv12 -video_size WxH -framerate N
//              -i pipe:0 <encoder ...> -f rtsp rtsp://127.0.0.1:8554/<id>
//
//   videonode-composer ... --scm-out /tmp/vn-bus-composer-<id>.sock
//   videonode-sink --socket /tmp/vn-bus-composer-<id>.sock
//     | ffmpeg -f rawvideo -pix_fmt bgra -s WxH -framerate N -i pipe:0
//              <encoder ...> -f rtsp ...
//
// Output format is auto-selected from the first frame's fourcc:
//   - NV12            → raw NV12 bytes (Y plane, then UV plane).
//   - BGRA / ARGB     → raw bytes per frame, no header.
//
// The first frame announces dims + format on stderr so callers can size
// the downstream ffmpeg invocation accordingly.

#include "src/common/log_levels.hpp"
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
#include <span>
#include <string>
#include <sys/ioctl.h>
#include <sys/mman.h>
#include <thread>
#include <unistd.h>

#ifdef __x86_64__
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
    struct dma_buf_sync sync {};
    sync.flags = DMA_BUF_SYNC_START | DMA_BUF_SYNC_READ;
    ::ioctl(fd, DMA_BUF_IOCTL_SYNC, &sync);
}

void dmabuf_sync_end(int fd) {
    struct dma_buf_sync sync {};
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
__attribute__((target("sse4.1")))
void streaming_memcpy(uint8_t* dst, const uint8_t* src, size_t len) {
#ifdef __x86_64__
    auto* d = reinterpret_cast<__m128i*>(dst);
    auto* s = const_cast<__m128i*>(reinterpret_cast<const __m128i*>(src));
    size_t n = len / 16;
    while (n >= 4) {
        __m128i r0 = _mm_stream_load_si128(s + 0);
        __m128i r1 = _mm_stream_load_si128(s + 1);
        __m128i r2 = _mm_stream_load_si128(s + 2);
        __m128i r3 = _mm_stream_load_si128(s + 3);
        _mm_store_si128(d + 0, r0);
        _mm_store_si128(d + 1, r1);
        _mm_store_si128(d + 2, r2);
        _mm_store_si128(d + 3, r3);
        s += 4;
        d += 4;
        n -= 4;
    }
    while (n-- > 0) {
        _mm_store_si128(d++, _mm_stream_load_si128(s++));
    }
    size_t tail = len & 15;
    if (tail > 0)
        std::memcpy(reinterpret_cast<uint8_t*>(d), reinterpret_cast<const uint8_t*>(s), tail);
    _mm_sfence();
#else
    std::memcpy(dst, src, len);
#endif
}

void copy_plane(uint8_t* dst, const uint8_t* src, size_t width, size_t pitch, size_t rows) {
    if (pitch == width) {
        streaming_memcpy(dst, src, width * rows);
    } else {
        for (size_t r = 0; r < rows; ++r)
            streaming_memcpy(dst + r * width, src + r * pitch, width);
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
    const auto* y_base = static_cast<const uint8_t*>(y_map) + v.plane0_offset;

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
        uv_base = static_cast<const uint8_t*>(uv_map) + v.plane1_offset;
    } else {
        size_t uv_off = v.plane1_offset ? v.plane1_offset : v.plane0_offset + y_pitch * height;
        uv_base = static_cast<const uint8_t*>(y_map) + uv_off;
    }

    // Copy from (potentially uncached) dma-buf into the cached scratch
    // buffer in one pass, then release the mmap before writing to the pipe.
    copy_plane(scratch, y_base, width, y_pitch, height);
    copy_plane(scratch + width * height, uv_base, width, uv_pitch, uv_rows);

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

bool emit_frame_raw_bgra(const scm_rights_source::FrameView& v) {
    if (v.fd < 0 || v.width <= 0 || v.height <= 0)
        return true;
    const size_t row_bytes = size_t(v.width) * 4;
    const size_t stride = v.plane0_pitch ? v.plane0_pitch : row_bytes;
    if (stride < row_bytes) {
        vn::log::error("videonode-sink: BGRA stride %zu < row_bytes %zu (width=%d); dropping frame",
                       stride, row_bytes, v.width);
        return true;
    }
    const size_t frame_bytes = row_bytes * v.height;
    uint8_t* scratch = aligned_scratch(frame_bytes);

    const size_t map_size = v.plane0_offset + stride * v.height;
    void* m = ::mmap(nullptr, map_size, PROT_READ, MAP_SHARED, v.fd, 0);
    if (m == MAP_FAILED) {
        vn::log::error("videonode-sink: mmap fd=%d failed: %s", v.fd, strerror(errno));
        return true;
    }
    dmabuf_sync_start(v.fd);
    const auto* base = static_cast<const uint8_t*>(m) + v.plane0_offset;
    copy_plane(scratch, base, row_bytes, stride, size_t(v.height));
    dmabuf_sync_end(v.fd);
    ::munmap(m, map_size);

    bool ok = write_full(STDOUT_FILENO, std::span(scratch, frame_bytes));
    if (!ok) {
        vn::log::info("videonode-sink: stdout closed, exiting");
        return false;
    }
    return true;
}

bool is_nv12_format(const std::string& fourcc) {
    return fourcc.empty() || fourcc == "NV12";
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
            "Output mode is auto-selected from the first frame's fourcc:\n"
            "  NV12              → raw NV12 bytes (Y + UV planes).\n"
            "                      Pipe to `ffmpeg -f rawvideo -pix_fmt nv12 -video_size WxH\n"
            "                      -framerate N -i pipe:0 ...`.\n"
            "  BGRA / ARGB       → raw bytes per frame (no header).\n"
            "                      Pipe to `ffmpeg -f rawvideo -pix_fmt bgra -s WxH\n"
            "                      -framerate N -i pipe:0 ...`.\n"
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
    bool nv12_mode = true;
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
            nv12_mode = is_nv12_format(v.format);
            vn::log::info("videonode-sink: streaming raw %dx%d (%s fourcc=%s) from %s", v.width,
                          v.height, nv12_mode ? "NV12" : "BGRA",
                          v.format.empty() ? "NV12" : v.format.c_str(), a.socket_path.c_str());
            announced = true;
            announced_w = v.width;
            announced_h = v.height;
        }
        if (a.verbose) {
            vn::log::info("videonode-sink: frame_idx=%llu fd=%d",
                          static_cast<unsigned long long>(v.frame_idx), v.fd);
        }
        last_idx = v.frame_idx;
        bool ok = nv12_mode ? emit_frame_nv12_raw(v) : emit_frame_raw_bgra(v);
        if (!ok)
            break;
    }

    src.stop();
    vn::log::info("videonode-sink: shutdown");
    return 0;
}
