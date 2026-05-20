// videonode-source — V4L2 capture sidecar with always-on placeholder.
//
// Main loop is event-driven: poll(fd, POLLIN|POLLPRI) wakes us on either
// a ready frame or a V4L2 event. SourceProbe consumes those events plus
// DQBUF results to compute a health state (Live / Transitioning /
// NoCable / NoLock / Gone / Probing). The broadcaster ticks at a fixed
// rate and chooses what to send:
//
//   Live           → newest real-frame fd
//                    raw V4L2 formats: RGA-CSC'd to NV12 into out_ring
//                    MJPEG: MPP-HW decode (rig) → MPP-pool fd, or
//                           TurboJPEG SW decode (host) → out_ring fd
//   Transitioning  → last-good real-frame fd, re-broadcast with new idx
//                    (driver renegotiation gap — content didn't really
//                     change, downstream sees no flicker)
//   NoCable/NoLock/Gone/Probing → painted placeholder with status text
//
// There are no time-based "stale" thresholds anywhere. State is whatever
// the driver tells us via V4L2 events and ctrl reads.

#include "v4l2_capture.hpp"
#include "rga_csc.hpp"
#include "placeholder_painter.hpp"
#include "source_probe.hpp"
#include "scm_rights_producer.hpp"
#include "dmabuf_msg.hpp"
#include "dma_heap.hpp"
#include "jpeg_dec.hpp"
#include "jpeg_dec_turbo.hpp"
#include "mpp_jpeg_dec.hpp"
#include "version.hpp"

#include <atomic>
#include <cerrno>
#include <chrono>
#include <csignal>
#include <cstdio>
#include <cstdlib>
#include <cstring>
#include <linux/videodev2.h>
#include <memory>
#include <poll.h>
#include <string>
#include <sys/mman.h>
#include <sys/prctl.h>
#include <thread>
#include <unistd.h>
#include <vector>

namespace {

std::atomic<bool> g_running{true};
void on_signal(int) {
    g_running.store(false);
}

struct Args {
    std::string device = "/dev/video0";
    std::string in_format;
    int in_width = 0;
    int in_height = 0;
    int in_fps = 0;
    int buffers = 4;
    std::string out_socket = "/tmp/videonode-source.sock";
    int max_consumers = 16;
    int run_seconds = 0;
    int broadcast_fps = 60;
    int placeholder_w = 1920;
    int placeholder_h = 1080;
};

uint32_t fourcc_(const std::string& s) {
    if (s.size() != 4)
        return 0;
    return uint32_t(uint8_t(s[0])) | (uint32_t(uint8_t(s[1])) << 8) |
           (uint32_t(uint8_t(s[2])) << 16) | (uint32_t(uint8_t(s[3])) << 24);
}
uint32_t v4l2_pix_fmt_(const std::string& s) {
    if (s == "NV12")
        return V4L2_PIX_FMT_NV12;
    if (s == "NV16")
        return V4L2_PIX_FMT_NV16;
    if (s == "NV24")
        return V4L2_PIX_FMT_NV24;
    if (s == "BGR3" || s == "BG24")
        return V4L2_PIX_FMT_BGR24;
    if (s == "YUYV")
        return V4L2_PIX_FMT_YUYV;
    if (s == "UYVY")
        return V4L2_PIX_FMT_UYVY;
    if (s == "MJPG" || s == "MJPEG")
        return V4L2_PIX_FMT_MJPEG;
    return fourcc_(s);
}
bool v4l2_to_rga_(uint32_t v4l2_fmt, rga::PixelFormat& out, std::string& name) {
    switch (v4l2_fmt) {
    case V4L2_PIX_FMT_NV12:
        out = rga::PixelFormat::Nv12;
        name = "NV12";
        return true;
    case V4L2_PIX_FMT_NV16:
        out = rga::PixelFormat::Nv16;
        name = "NV16";
        return true;
    case V4L2_PIX_FMT_NV24:
        out = rga::PixelFormat::Nv24;
        name = "NV24";
        return true;
    case V4L2_PIX_FMT_BGR24:
        out = rga::PixelFormat::Bgr3;
        name = "BGR3";
        return true;
    case V4L2_PIX_FMT_YUYV:
        out = rga::PixelFormat::Yuyv;
        name = "YUYV";
        return true;
    case V4L2_PIX_FMT_UYVY:
        out = rga::PixelFormat::Uyvy;
        name = "UYVY";
        return true;
    }
    return false;
}
bool maybe_renegotiate_to_rga_friendly_(v4l2::Streamer& cap, v4l2::StreamFormat& cur) {
    if (cur.pixel_format != V4L2_PIX_FMT_NV24)
        return false;
    v4l2::StreamFormat want = cur;
    want.pixel_format = V4L2_PIX_FMT_NV16;
    if (!cap.set_format(want))
        return false;
    return cap.get_format(cur);
}

enum class DecodeMode {
    Rga,   // RGA color-space-convert raw V4L2 format → NV12 into out_ring
    Mjpeg, // JPEG decode (MPP HW on rig, TurboJPEG SW on host) → NV12
};

// CaptureSession bundles V4L2 streamer + decoder + output buffers.
//
// RGA path: out_ring is filled by RGA on each DQBUF.
// MJPEG / MPP backend: out_ring is unused (MPP owns its pool).
// MJPEG / TurboJPEG backend: out_ring is mmap'd writable (out_maps) and
//                            the decoder writes NV12 bytes directly into
//                            the next slot.
struct CaptureSession {
    bool active = false;
    v4l2::Streamer cap;
    std::vector<dmaheap::Buffer> out_ring;
    rga::PixelFormat src_fmt = rga::PixelFormat::Nv12;
    std::string src_fmt_name;
    int width = 0;
    int height = 0;

    DecodeMode mode = DecodeMode::Rga;

    // MJPEG path:
    std::unique_ptr<jpeg_dec::JpegDec> jpeg;
    bool using_mpp = false;     // log-only
    std::vector<void*> in_maps; // V4L2 capture buffer mmaps (JPEG bytes)
    std::vector<size_t> in_map_sizes;
    std::vector<void*> out_maps; // writable mmaps into out_ring (TurboJPEG)
    std::vector<size_t> out_map_sizes;
};

void teardown_session_(CaptureSession& s) {
    if (s.jpeg)
        s.jpeg.reset();
    for (size_t i = 0; i < s.out_maps.size(); ++i) {
        if (s.out_maps[i] && s.out_maps[i] != MAP_FAILED)
            ::munmap(s.out_maps[i], s.out_map_sizes[i]);
    }
    s.out_maps.clear();
    s.out_map_sizes.clear();
    s.in_maps.clear();
    s.in_map_sizes.clear();
    s.out_ring.clear();
    s.cap.close(); // unmaps V4L2 in_maps inside Streamer
    s.active = false;
    s.width = 0;
    s.height = 0;
    s.src_fmt_name.clear();
    s.mode = DecodeMode::Rga;
    s.using_mpp = false;
}

bool try_open_capture(CaptureSession& s, const Args& a) {
    teardown_session_(s);
    if (!s.cap.open(a.device))
        return false;

    v4l2::StreamFormat cur;
    if (a.in_format.empty()) {
        if (!s.cap.get_format(cur)) {
            s.cap.close();
            return false;
        }
        maybe_renegotiate_to_rga_friendly_(s.cap, cur);
    } else {
        cur.pixel_format = v4l2_pix_fmt_(a.in_format);
        cur.width = a.in_width > 0 ? a.in_width : a.placeholder_w;
        cur.height = a.in_height > 0 ? a.in_height : a.placeholder_h;
        cur.fps = a.in_fps;
        if (!s.cap.set_format(cur)) {
            s.cap.close();
            return false;
        }
        if (!s.cap.get_format(cur)) {
            s.cap.close();
            return false;
        }
    }

    if (cur.pixel_format == V4L2_PIX_FMT_MJPEG) {
        s.mode = DecodeMode::Mjpeg;
        s.src_fmt_name = "MJPG";
    } else {
        s.mode = DecodeMode::Rga;
        if (!v4l2_to_rga_(cur.pixel_format, s.src_fmt, s.src_fmt_name)) {
            s.cap.close();
            return false;
        }
    }
    s.width = int(cur.width);
    s.height = int(cur.height);
    if (s.width <= 0 || s.height <= 0) {
        s.cap.close();
        return false;
    }

    std::vector<v4l2::BufferRef> _ignored;
    if (!s.cap.request_buffers(a.buffers, _ignored)) {
        s.cap.close();
        return false;
    }
    if (!s.cap.export_all_planes()) {
        s.cap.close();
        return false;
    }
    for (const auto& b : s.cap.buffers()) {
        if (!s.cap.queue_buffer(b.index)) {
            s.cap.close();
            return false;
        }
    }
    if (!s.cap.stream_on()) {
        s.cap.close();
        return false;
    }

    const size_t out_size = size_t(s.width) * s.height * 3 / 2;

    if (s.mode == DecodeMode::Mjpeg) {
        // mmap each V4L2 capture buffer for CPU read of variable-length
        // JPEG payloads. MJPEG is single-plane only — mmap_buffer asserts.
        for (const auto& b : s.cap.buffers()) {
            void* ptr = nullptr;
            size_t sz = 0;
            if (!s.cap.mmap_buffer(b.index, ptr, sz)) {
                s.cap.close();
                return false;
            }
            s.in_maps.push_back(ptr);
            s.in_map_sizes.push_back(sz);
        }

        // Probe MPP first; on host the rockchip_stubs make mpp_create fail
        // and we fall through to TurboJPEG.
        auto mpp = std::make_unique<mpp_jpeg_dec::MppJpegDec>();
        if (mpp->init(s.width, s.height)) {
            s.jpeg = std::move(mpp);
            s.using_mpp = true;
            fprintf(stderr, "videonode-source: MJPEG backend = MPP (HW)\n");
        } else {
            // TurboJPEG fallback. Allocate out_ring + mmap each slot
            // writable, hand the (fd, ptr) pairs to the decoder.
            for (int i = 0; i < a.buffers; ++i) {
                dmaheap::Buffer b = dmaheap::alloc("system-uncached", out_size);
                if (!b.valid()) {
                    s.cap.close();
                    s.out_ring.clear();
                    return false;
                }
                s.out_ring.push_back(std::move(b));
            }
            std::vector<jpeg_dec::TurboJpegDec::Slot> slots;
            slots.reserve(s.out_ring.size());
            for (auto& buf : s.out_ring) {
                void* m = ::mmap(nullptr, out_size, PROT_READ | PROT_WRITE, MAP_SHARED, buf.fd, 0);
                if (m == MAP_FAILED) {
                    fprintf(stderr, "videonode-source: mmap out_ring fd=%d: %s\n", buf.fd,
                            strerror(errno));
                    s.cap.close();
                    return false;
                }
                s.out_maps.push_back(m);
                s.out_map_sizes.push_back(out_size);
                slots.push_back({buf.fd, static_cast<uint8_t*>(m)});
            }
            auto tj = std::make_unique<jpeg_dec::TurboJpegDec>();
            if (!tj->init(s.width, s.height, std::move(slots))) {
                s.cap.close();
                return false;
            }
            s.jpeg = std::move(tj);
            s.using_mpp = false;
            fprintf(stderr, "videonode-source: MJPEG backend = TurboJPEG (SW)\n");
        }
    } else {
        // RGA path: out_ring holds NV12 dma-heap buffers RGA writes into.
        for (int i = 0; i < a.buffers; ++i) {
            dmaheap::Buffer b = dmaheap::alloc("system-uncached", out_size);
            if (!b.valid()) {
                s.cap.close();
                s.out_ring.clear();
                return false;
            }
            s.out_ring.push_back(std::move(b));
        }
    }

    s.active = true;
    fprintf(
        stderr,
        "videonode-source: capture ready — %dx%d %s, %d v4l2 buffers (multiplanar=%d, mode=%s)\n",
        s.width, s.height, s.src_fmt_name.c_str(), int(s.cap.buffers().size()),
        int(s.cap.multiplanar()),
        s.mode == DecodeMode::Mjpeg ? (s.using_mpp ? "mjpeg-mpp" : "mjpeg-turbojpeg") : "rga");
    return true;
}

struct PlaceholderRing {
    int width = 0;
    int height = 0;
    std::vector<dmaheap::Buffer> bufs;
    std::vector<void*> maps;
    std::vector<size_t> sizes;
    int next = 0;
    uint64_t tick_idx = 0;

    bool init(int w, int h, const std::string& device_path) {
        width = w;
        height = h;
        const size_t sz = size_t(w) * h * 3 / 2;
        for (int i = 0; i < 2; ++i) {
            dmaheap::Buffer b = dmaheap::alloc("system-uncached", sz);
            if (!b.valid())
                return false;
            void* m = ::mmap(nullptr, sz, PROT_READ | PROT_WRITE, MAP_SHARED, b.fd, 0);
            if (m == MAP_FAILED)
                return false;
            placeholder_painter::paint_base(static_cast<uint8_t*>(m), w, h, device_path.c_str());
            bufs.push_back(std::move(b));
            maps.push_back(m);
            sizes.push_back(sz);
        }
        return true;
    }
    int paint_and_pick_fd(uint64_t wallclock_ms, const char* status) {
        ++tick_idx;
        int idx = next;
        next = (next + 1) % int(bufs.size());
        placeholder_painter::paint_tick(static_cast<uint8_t*>(maps[idx]), width, height, tick_idx,
                                        wallclock_ms, status);
        return bufs[idx].fd;
    }
    void destroy() {
        for (size_t i = 0; i < bufs.size(); ++i) {
            if (maps[i] && maps[i] != MAP_FAILED)
                ::munmap(maps[i], sizes[i]);
        }
        bufs.clear();
        maps.clear();
        sizes.clear();
    }
};

void print_help(const Args& d) {
    printf("videonode-source — V4L2 capture → (RGA-CSC | JPEG-decode) → NV12 dma-buf → SCM_RIGHTS\n"
           "  with event-driven placeholder when the source is absent or in flux.\n"
           "\n"
           "  --device PATH                 /dev/videoN (default %s)\n"
           "  --in-format FMT               NV24/NV16/NV12/BGR3/YUYV/UYVY/MJPG (empty = auto)\n"
           "  --in-width W / --in-height H  geometry for explicit format\n"
           "  --in-fps N                    requested capture rate\n"
           "  --buffers N                   V4L2 ring size (default %d)\n"
           "  --out-socket PATH             Unix socket to publish NV12 dma-bufs on (default %s)\n"
           "  --max-consumers N             soft cap on concurrent consumers (default %d)\n"
           "  --seconds N                   stop after N seconds (default %d = until SIGINT)\n"
           "  --broadcast-fps N             publish rate (default %d)\n"
           "  --placeholder-w W             placeholder canvas width  (default %d)\n"
           "  --placeholder-h H             placeholder canvas height (default %d)\n",
           d.device.c_str(), d.buffers, d.out_socket.c_str(), d.max_consumers, d.run_seconds,
           d.broadcast_fps, d.placeholder_w, d.placeholder_h);
}

bool parse_args(int argc, char** argv, Args& a) {
    auto eat = [&](int& i) -> const char* {
        if (i + 1 >= argc)
            return nullptr;
        return argv[++i];
    };
    for (int i = 1; i < argc; ++i) {
        std::string s = argv[i];
        if (s == "--device") {
            const char* v = eat(i);
            if (!v)
                return false;
            a.device = v;
        } else if (s == "--in-format") {
            const char* v = eat(i);
            if (!v)
                return false;
            a.in_format = v;
        } else if (s == "--in-width") {
            const char* v = eat(i);
            if (!v)
                return false;
            a.in_width = atoi(v);
        } else if (s == "--in-height") {
            const char* v = eat(i);
            if (!v)
                return false;
            a.in_height = atoi(v);
        } else if (s == "--in-fps") {
            const char* v = eat(i);
            if (!v)
                return false;
            a.in_fps = atoi(v);
        } else if (s == "--buffers") {
            const char* v = eat(i);
            if (!v)
                return false;
            a.buffers = atoi(v);
        } else if (s == "--out-socket") {
            const char* v = eat(i);
            if (!v)
                return false;
            a.out_socket = v;
        } else if (s == "--max-consumers") {
            const char* v = eat(i);
            if (!v)
                return false;
            a.max_consumers = atoi(v);
        } else if (s == "--seconds") {
            const char* v = eat(i);
            if (!v)
                return false;
            a.run_seconds = atoi(v);
        } else if (s == "--broadcast-fps") {
            const char* v = eat(i);
            if (!v)
                return false;
            a.broadcast_fps = atoi(v);
        } else if (s == "--placeholder-w") {
            const char* v = eat(i);
            if (!v)
                return false;
            a.placeholder_w = atoi(v);
        } else if (s == "--placeholder-h") {
            const char* v = eat(i);
            if (!v)
                return false;
            a.placeholder_h = atoi(v);
        } else if (s == "-h" || s == "--help") {
            print_help(a);
            exit(0);
        } else if (s == "--version") {
            printf("videonode-source %s\n", vn::kVersion);
            exit(0);
        } else {
            fprintf(stderr, "videonode-source: unknown flag %s\n", s.c_str());
            return false;
        }
    }
    return true;
}

uint64_t now_ms() {
    using namespace std::chrono;
    return duration_cast<milliseconds>(steady_clock::now().time_since_epoch()).count();
}

void broadcast_nv12(scm_rights_producer::ScmRightsProducer& prod, const jpeg_dec::DecodedNv12& d,
                    uint64_t frame_idx) {
    dmabuf_msg::Header h_;
    h_.slot_index = 0;
    h_.width = uint32_t(d.width);
    h_.height = uint32_t(d.height);
    h_.format = "NV12";
    h_.plane_pitches = {d.y_pitch, d.uv_pitch};
    h_.plane_offsets = {d.y_offset, d.uv_offset};
    h_.frame_idx = frame_idx;
    prod.broadcast(h_, {d.fd, d.fd});
}

// Thin shim for the RGA + placeholder paths that produce tight NV12 (no
// stride padding). MJPEG-MPP can't use this because it returns padded
// strides; it builds its own DecodedNv12.
void broadcast_frame(scm_rights_producer::ScmRightsProducer& prod, int fd, int w, int h,
                     uint64_t frame_idx) {
    jpeg_dec::DecodedNv12 d;
    d.fd = fd;
    d.width = w;
    d.height = h;
    d.y_pitch = uint32_t(w);
    d.uv_pitch = uint32_t(w);
    d.y_offset = 0;
    d.uv_offset = uint32_t(w) * uint32_t(h);
    broadcast_nv12(prod, d, frame_idx);
}

} // namespace

int main(int argc, char** argv) {
    // stderr defaults to block-buffered when redirected to a file (which
    // the supervisor does: `>log 2>&1`). Force line-buffered so each log
    // line is visible to `tail -f` immediately instead of waiting for a
    // 4 KiB chunk to accumulate.
    ::setvbuf(stderr, nullptr, _IOLBF, 0);

    Args a;
    if (!parse_args(argc, argv, a))
        return 2;
    std::signal(SIGINT, on_signal);
    std::signal(SIGTERM, on_signal);
    std::signal(SIGPIPE, SIG_IGN);
    ::prctl(PR_SET_PDEATHSIG, SIGTERM);
    if (::getppid() == 1)
        return 0;

    PlaceholderRing ph;
    if (!ph.init(a.placeholder_w, a.placeholder_h, a.device)) {
        fprintf(stderr, "videonode-source: failed to allocate placeholder ring\n");
        return 1;
    }
    fprintf(stderr, "videonode-source: placeholder %dx%d ready\n", a.placeholder_w,
            a.placeholder_h);

    scm_rights_producer::ScmRightsProducer prod;
    scm_rights_producer::InitParams pp;
    pp.socket_path = a.out_socket;
    pp.max_consumers = a.max_consumers;
    if (!prod.init(pp) || !prod.start())
        return 1;

    CaptureSession cap;
    source_probe::SourceProbe probe(cap.cap);
    if (try_open_capture(cap, a)) {
        probe.attach();
    } else {
        fprintf(stderr, "videonode-source: capture not ready at startup\n");
    }

    using clock = std::chrono::steady_clock;
    const auto broadcast_period =
        std::chrono::nanoseconds(1'000'000'000LL / std::max(1, a.broadcast_fps));
    auto loop_start = clock::now();
    auto next_broadcast = clock::now();

    uint64_t real_frame_idx = 0;
    int last_good_out_fd = -1;
    int last_good_w = 0, last_good_h = 0;
    source_probe::Health prev_health = source_probe::Health::Probing;
    bool need_reinit = !cap.active;
    // Power-present poll backstop: re-read the control once per second in
    // case the driver doesn't fire SOURCE_CHANGE on cable unplug. Cheap
    // (one VIDIOC_G_CTRL ioctl); guards against event-only blindspots.
    auto next_power_poll = clock::now();

    while (g_running.load()) {
        if (a.run_seconds > 0 && clock::now() - loop_start > std::chrono::seconds(a.run_seconds))
            break;

        // Reinit capture if we lost it.
        if (need_reinit) {
            if (try_open_capture(cap, a)) {
                probe.attach();
                need_reinit = false;
            }
            // even if reinit failed we proceed — placeholder still ticks
        }

        // poll() with a timeout that wakes us up in time for next broadcast.
        // Negative deltas clamp to 0.
        auto until_next = next_broadcast - clock::now();
        int poll_timeout_ms =
            int(std::chrono::duration_cast<std::chrono::milliseconds>(until_next).count());
        if (poll_timeout_ms < 0)
            poll_timeout_ms = 0;
        if (poll_timeout_ms > 100)
            poll_timeout_ms = 100;

        if (cap.active) {
            pollfd pfd{cap.cap.fd(), short(POLLIN | POLLPRI), 0};
            int pr = ::poll(&pfd, 1, poll_timeout_ms);
            if (pr > 0) {
                if (pfd.revents & POLLPRI) {
                    std::vector<v4l2_event> evs;
                    if (cap.cap.drain_events_typed(evs)) {
                        bool need_restart = false;
                        for (const auto& e : evs) {
                            probe.note_event(e);
                            if (e.type == V4L2_EVENT_SOURCE_CHANGE)
                                need_restart = true;
                        }
                        if (need_restart) {
                            if (cap.cap.restart_streaming()) {
                                probe.note_streaming_restarted();
                            } else {
                                teardown_session_(cap);
                                need_reinit = true;
                            }
                        }
                    }
                }
                if (pfd.revents & POLLIN) {
                    v4l2::DequeuedFrame df;
                    if (cap.cap.dequeue_buffer(0, df)) {
                        probe.note_dqbuf_success();
                        bool ok = false;
                        jpeg_dec::DecodedNv12 decoded;
                        if (cap.mode == DecodeMode::Rga) {
                            rga::ConvertParams src_p, dst_p;
                            src_p.fd = cap.cap.buffers()[df.index].primary_dma_buf();
                            src_p.fmt = cap.src_fmt;
                            src_p.width = cap.width;
                            src_p.height = cap.height;
                            int out_fd = cap.out_ring[df.index % cap.out_ring.size()].fd;
                            dst_p.fd = out_fd;
                            dst_p.fmt = rga::PixelFormat::Nv12;
                            dst_p.width = cap.width;
                            dst_p.height = cap.height;
                            if (rga::convert(src_p, dst_p)) {
                                decoded.fd = out_fd;
                                decoded.width = cap.width;
                                decoded.height = cap.height;
                                decoded.y_pitch = uint32_t(cap.width);
                                decoded.uv_pitch = uint32_t(cap.width);
                                decoded.y_offset = 0;
                                decoded.uv_offset = uint32_t(cap.width) * uint32_t(cap.height);
                                ok = true;
                            }
                        } else { // DecodeMode::Mjpeg
                            if (df.index < cap.in_maps.size() && df.bytesused > 0) {
                                const uint8_t* jpeg =
                                    static_cast<const uint8_t*>(cap.in_maps[df.index]);
                                ok = cap.jpeg->decode(jpeg, df.bytesused, decoded);
                            }
                        }
                        if (ok) {
                            ++real_frame_idx;
                            broadcast_nv12(prod, decoded, real_frame_idx);
                            last_good_out_fd = decoded.fd;
                            last_good_w = decoded.width;
                            last_good_h = decoded.height;
                            // Push the next-broadcast forward so a real
                            // frame's broadcast counts as the tick.
                            next_broadcast = clock::now() + broadcast_period;
                        }
                        cap.cap.queue_buffer(df.index);
                    } else {
                        int e = errno;
                        if (e != ETIMEDOUT && e != EAGAIN) {
                            probe.note_dqbuf_failure(e);
                            if (e == ENODEV) {
                                teardown_session_(cap);
                                last_good_out_fd = -1;
                                need_reinit = true;
                            }
                        }
                    }
                }
            }
        } else {
            // No capture; just sleep to next broadcast.
            std::this_thread::sleep_until(next_broadcast);
        }

        // Log state transitions regardless of whether this iteration
        // already broadcast a real frame — otherwise Live transitions go
        // unlogged whenever DQBUFs keep arriving inside the broadcast period.
        if (clock::now() >= next_power_poll) {
            probe.refresh_power_present();
            next_power_poll = clock::now() + std::chrono::seconds(1);
        }
        source_probe::Health h = probe.health();
        if (h != prev_health) {
            fprintf(stderr, "videonode-source: state -> %s\n", source_probe::status_text(h));
            prev_health = h;
        }

        // Time to broadcast a tick?
        if (clock::now() < next_broadcast)
            continue;

        if (h == source_probe::Health::Live) {
            // already broadcast via DQBUF path; nothing extra to do here.
            next_broadcast += broadcast_period;
            continue;
        }
        if (h == source_probe::Health::Transitioning && last_good_out_fd >= 0) {
            // Re-broadcast last good real frame with fresh sequence.
            ++real_frame_idx;
            broadcast_frame(prod, last_good_out_fd, last_good_w, last_good_h, real_frame_idx);
        } else {
            // Probing / NoCable / NoLock / Gone / Transitioning-without-history.
            int fd = ph.paint_and_pick_fd(now_ms(), source_probe::status_text(h));
            broadcast_frame(prod, fd, ph.width, ph.height, ph.tick_idx);
        }
        next_broadcast += broadcast_period;
        if (next_broadcast < clock::now()) {
            next_broadcast = clock::now() + broadcast_period;
        }
    }

    fprintf(stderr, "videonode-source: shutting down (real=%llu placeholder=%llu)\n",
            static_cast<unsigned long long>(real_frame_idx),
            static_cast<unsigned long long>(ph.tick_idx));
    prod.stop();
    if (cap.active) {
        cap.cap.stream_off();
        teardown_session_(cap);
    }
    ph.destroy();
    return 0;
}
