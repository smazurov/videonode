// SourceOrchestrator implementation. See orchestrator.hpp.
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

#include "src/source/orchestrator.hpp"

#include "src/capture/source_probe.hpp"
#include "src/render/nv12_buf.hpp"
#include "src/render/placeholder_painter.hpp"
#include "src/rpc/control_channel.hpp"
#include "src/rpc/jsonrpc_msg.hpp"
#include "src/source/broadcast.hpp"
#include "src/source/capture_session.hpp"
#include "version.hpp"
#if defined(HAVE_GBM) && !defined(HAVE_RGA)
#include "src/render/egl_ctx.hpp"
#endif

#include <atomic>
#include <cerrno>
#include <chrono>
#include <cstdio>
#include <cstdlib>
#include <cstring>
#include <linux/videodev2.h>
#include <poll.h>
#include <span>
#include <string>
#include <string_view>
#include <thread>
#include <unistd.h>
#include <vector>

namespace source {

namespace {

struct PlaceholderRing {
    int width = 0;
    int height = 0;
    std::vector<nv12_buf::Buffer> bufs;
    std::vector<uint8_t> stage_; // tightly-packed CPU NV12 (W*H*1.5)
    int next = 0;
    uint64_t tick_idx = 0;

    bool init(nv12_buf::Allocator& alloc, int w, int h, const std::string& device_path) {
        width = w;
        height = h;
        const size_t tight = size_t(w) * h * 3 / 2;
        stage_.assign(tight, 0);
        placeholder_painter::paint_base(stage_, w, h, device_path.c_str());
        for (int i = 0; i < 2; ++i) {
            nv12_buf::Buffer b = alloc.alloc(w, h);
            if (!b.valid())
                return false;
            auto m = nv12_buf::map_rw(b);
            if (!m.y || !m.uv)
                return false;
            const uint8_t* src_y = stage_.data();
            const uint8_t* src_uv = stage_.data() + size_t(w) * h;
            for (int y = 0; y < h; ++y)
                std::memcpy(static_cast<uint8_t*>(m.y) + y * b.y_pitch, src_y + y * w, w);
            for (int y = 0; y < h / 2; ++y)
                std::memcpy(static_cast<uint8_t*>(m.uv) + y * b.uv_pitch, src_uv + y * w, w);
            nv12_buf::unmap(b);
            bufs.push_back(std::move(b));
        }
        return true;
    }
    // Returns a reference to the slot that holds the freshest placeholder
    // frame; caller passes it to broadcast_frame to forward both fds +
    // plane offsets.
    nv12_buf::Buffer& paint_and_pick(uint64_t wallclock_ms, const char* status) {
        ++tick_idx;
        int idx = next;
        next = (next + 1) % int(bufs.size());
        placeholder_painter::paint_tick(stage_, width, height, tick_idx, wallclock_ms, status);
        nv12_buf::Buffer& b = bufs[idx];
        auto m = nv12_buf::map_rw(b);
        if (m.y) {
            for (int y = 0; y < height; ++y)
                std::memcpy(static_cast<uint8_t*>(m.y) + y * b.y_pitch,
                            stage_.data() + y * width, width);
        }
        nv12_buf::unmap(b);
        return b;
    }
    void destroy() {
        bufs.clear();
        stage_.clear();
    }
};

} // namespace

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
           "  --placeholder-h H             placeholder canvas height (default %d)\n"
           "  --ctl-connect PATH            daemon control UDS to dial (omit to disable)\n"
           "  --device-id ID                stable device ID for control-plane identify\n",
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
        } else if (s == "--ctl-connect") {
            const char* v = eat(i);
            if (!v)
                return false;
            a.ctl_connect = v;
        } else if (s == "--device-id") {
            const char* v = eat(i);
            if (!v)
                return false;
            a.device_id = v;
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

int Run(const Args& a_in, std::atomic<bool>& running) {
    // Local mutable copy: set_format requests at runtime mutate in_format/
    // width/height/fps for the reinit-with-new-args path below.
    Args a = a_in;

    // NV12 output allocator. On rig (HAVE_RGA) this is stateless and
    // backed by dma_heap (single bo); on Fedora / Mesa hosts (HAVE_GBM,
    // no RGA) we open an EglCtx for its gbm_device and hand it to the
    // allocator (two-bo split for radeonsi compat).
#if defined(HAVE_GBM) && !defined(HAVE_RGA)
    egl_ctx::EglCtx alloc_ctx;
    if (!alloc_ctx.init(a.alloc_drm_device)) {
        fprintf(stderr, "videonode-source: failed to open DRM render node %s for GBM allocator\n",
                a.alloc_drm_device.c_str());
        return 1;
    }
    nv12_buf::Allocator allocator;
    if (!allocator.init(alloc_ctx.gbm())) {
        fprintf(stderr, "videonode-source: nv12_buf::Allocator::init failed\n");
        return 1;
    }
#else
    nv12_buf::Allocator allocator;
    if (!allocator.init()) {
        fprintf(stderr, "videonode-source: nv12_buf::Allocator::init failed\n");
        return 1;
    }
#endif

    PlaceholderRing ph;
    if (!ph.init(allocator, a.placeholder_w, a.placeholder_h, a.device)) {
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
    if (try_open_capture(cap, a, allocator)) {
        probe.attach();
    } else {
        fprintf(stderr, "videonode-source: capture not ready at startup\n");
    }

    // Control plane: dial the daemon if --ctl-connect was provided.
    // Without it, the sidecar runs standalone (e.g. from smoke scripts)
    // with no command/status channel.
    control_channel::ControlChannel ctl;
    bool ctl_enabled = !a.ctl_connect.empty() && !a.device_id.empty();
    bool need_reinit_for_format_change = false;
    if (ctl_enabled) {
        ctl.init(a.ctl_connect, a.device_id, vn::kVersion);
        ctl.set_command_handler(
            [&](const control_channel::IncomingRequest& req) -> control_channel::HandlerResponse {
                control_channel::HandlerResponse resp;
                if (req.method == "shutdown") {
                    running.store(false);
                    resp.ok = true;
                    return resp;
                }
                if (req.method == "get_status") {
                    // Caller fills in the snapshot below when it sends it
                    // back over the wire — we can't reach all the state
                    // we'd need here without yet more captures. Simpler:
                    // schedule a push and reply with an empty ack.
                    resp.ok = true;
                    return resp;
                }
                if (req.method == "set_format") {
                    // Parse params: {"fourcc":"YUYV","w":1920,"h":1080,"fps":30}
                    // Hand-roll the parse using jsonrpc_msg helpers.
                    using namespace jsonrpc_msg::parse;
                    std::string_view s = req.params_json;
                    size_t p = skip_ws(s, 0);
                    if (p >= s.size() || s[p] != '{') {
                        resp.ok = false;
                        resp.error_code = -32602;
                        resp.error_message = "params must be object";
                        return resp;
                    }
                    ++p;
                    std::string fourcc;
                    uint64_t w = 0, h = 0, fps = 0;
                    bool got_fourcc = false, got_w = false, got_h = false;
                    while (true) {
                        p = skip_ws(s, p);
                        if (p >= s.size()) {
                            resp.ok = false;
                            resp.error_code = -32602;
                            resp.error_message = "truncated params";
                            return resp;
                        }
                        if (s[p] == '}') {
                            ++p;
                            break;
                        }
                        std::string key;
                        size_t np = parse_string(s, p, key);
                        if (np == std::string::npos) {
                            resp.ok = false;
                            resp.error_code = -32602;
                            resp.error_message = "bad key";
                            return resp;
                        }
                        p = np;
                        p = skip_ws(s, p);
                        if (p >= s.size() || s[p] != ':') {
                            resp.ok = false;
                            resp.error_code = -32602;
                            resp.error_message = "expected ':'";
                            return resp;
                        }
                        ++p;
                        p = skip_ws(s, p);
                        if (key == "fourcc") {
                            np = parse_string(s, p, fourcc);
                            if (np == std::string::npos) {
                                resp.ok = false;
                                resp.error_code = -32602;
                                resp.error_message = "bad fourcc";
                                return resp;
                            }
                            got_fourcc = true;
                            p = np;
                        } else if (key == "w" || key == "h" || key == "fps") {
                            uint64_t v = 0;
                            np = parse_uint(s, p, v);
                            if (np == std::string::npos) {
                                resp.ok = false;
                                resp.error_code = -32602;
                                resp.error_message = "bad numeric field";
                                return resp;
                            }
                            if (key == "w") {
                                w = v;
                                got_w = true;
                            } else if (key == "h") {
                                h = v;
                                got_h = true;
                            } else {
                                fps = v;
                            }
                            p = np;
                        } else {
                            np = skip_value(s, p);
                            if (np == std::string::npos) {
                                resp.ok = false;
                                resp.error_code = -32602;
                                resp.error_message = "bad value";
                                return resp;
                            }
                            p = np;
                        }
                        p = skip_ws(s, p);
                        if (p < s.size() && s[p] == ',') {
                            ++p;
                            continue;
                        }
                        if (p < s.size() && s[p] == '}') {
                            ++p;
                            break;
                        }
                        resp.ok = false;
                        resp.error_code = -32602;
                        resp.error_message = "expected ',' or '}'";
                        return resp;
                    }
                    if (!got_fourcc || !got_w || !got_h) {
                        resp.ok = false;
                        resp.error_code = -32602;
                        resp.error_message = "missing required field (fourcc, w, h)";
                        return resp;
                    }
                    if (v4l2_pix_fmt_(fourcc) == 0) {
                        resp.ok = false;
                        resp.error_code = -32000;
                        resp.error_message = "unsupported fourcc";
                        return resp;
                    }
                    // Apply: stash new args, notify probe, mark for reinit.
                    a.in_format = fourcc;
                    a.in_width = int(w);
                    a.in_height = int(h);
                    a.in_fps = int(fps);
                    probe.note_format_change();
                    need_reinit_for_format_change = true;
                    fprintf(stderr,
                            "videonode-source: set_format requested: %s %ux%u@%u\n",
                            fourcc.c_str(), unsigned(w), unsigned(h), unsigned(fps));
                    resp.ok = true;
                    resp.result_json = "{\"applied\":true}";
                    return resp;
                }
                resp.ok = false;
                resp.error_code = -32601;
                resp.error_message = "method not found";
                return resp;
            });
        fprintf(stderr, "videonode-source: control plane → %s (id=%s)\n",
                a.ctl_connect.c_str(), a.device_id.c_str());
    }

    using clock = std::chrono::steady_clock;
    const auto broadcast_period =
        std::chrono::nanoseconds(1'000'000'000LL / std::max(1, a.broadcast_fps));
    auto loop_start = clock::now();
    auto next_broadcast = clock::now();

    uint64_t real_frame_idx = 0;
    uint32_t last_dqbuf_seq = 0;
    // Last fully-decoded real frame; re-broadcast during driver
    // renegotiation gaps so downstream sees stable content. fd == -1
    // means no good frame yet.
    jpeg_dec::DecodedNv12 last_good_decoded{};
    source_probe::Health prev_health = source_probe::Health::Probing;
    bool need_reinit = !cap.active;
    // Power-present poll backstop: re-read the control once per second in
    // case the driver doesn't fire SOURCE_CHANGE on cable unplug. Cheap
    // (one VIDIOC_G_CTRL ioctl); guards against event-only blindspots.
    auto next_power_poll = clock::now();
    auto next_status_heartbeat = clock::now();
    int prev_consumer_count = -1;

    while (running.load()) {
        if (a.run_seconds > 0 && clock::now() - loop_start > std::chrono::seconds(a.run_seconds))
            break;

        if (ctl_enabled) {
            ctl.maintain();
        }

        // Format-change reinit: synchronous teardown + reopen with the
        // new args. The probe was already marked Transitioning; the
        // last_good fd is invalidated because out_ring is reallocated.
        if (need_reinit_for_format_change) {
            last_good_decoded = {};
            if (try_open_capture(cap, a, allocator)) {
                probe.attach();
                need_reinit = false;
            } else {
                need_reinit = true;
            }
            need_reinit_for_format_change = false;
        }

        // Reinit capture if we lost it.
        if (need_reinit) {
            if (try_open_capture(cap, a, allocator)) {
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

        // Build pollset: capture fd (if active) + control-channel fd (if
        // connected). We keep slots stable so revents land where expected.
        std::vector<pollfd> pset;
        int cap_idx = -1;
        int ctl_idx = -1;
        if (cap.active) {
            pollfd pfd{};
            pfd.fd = cap.cap.fd();
            pfd.events = POLLIN | POLLPRI;
            cap_idx = int(pset.size());
            pset.push_back(pfd);
        }
        if (ctl_enabled && ctl.connected()) {
            ctl_idx = int(pset.size());
            ctl.add_to_poll(pset);
        }

        if (!pset.empty()) {
            int pr = ::poll(pset.data(), pset.size(), poll_timeout_ms);
            if (pr > 0 && ctl_idx >= 0) {
                ctl.handle_events(pset[ctl_idx].revents);
            }
            if (pr > 0 && cap_idx >= 0) {
                pollfd pfd = pset[cap_idx];
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
                        last_dqbuf_seq = df.sequence;
                        bool ok = false;
                        jpeg_dec::DecodedNv12 decoded;
                        if (cap.mode == DecodeMode::Rga) {
                            nv12_buf::Buffer& dst_buf =
                                cap.out_ring[df.index % cap.out_ring.size()];
                            csc::ConvertParams src_p, dst_p;
                            src_p.fd = cap.cap.buffers()[df.index].primary_dma_buf();
                            src_p.fmt = cap.src_fmt;
                            src_p.width = cap.width;
                            src_p.height = cap.height;
                            dst_p.fd = dst_buf.y_fd;
                            dst_p.fmt = csc::PixelFormat::Nv12;
                            dst_p.width = cap.width;
                            dst_p.height = cap.height;
                            if (csc::convert(src_p, dst_p)) {
                                decoded.fd = dst_buf.y_fd;
                                decoded.plane1_fd = dst_buf.uv_fd;
                                decoded.width = cap.width;
                                decoded.height = cap.height;
                                decoded.y_pitch = dst_buf.y_pitch;
                                decoded.uv_pitch = dst_buf.uv_pitch;
                                decoded.y_offset = dst_buf.y_offset;
                                decoded.uv_offset = dst_buf.uv_offset;
                                ok = true;
                            }
                        } else { // DecodeMode::Mjpeg
                            if (df.index < cap.in_maps.size() && df.bytesused > 0) {
                                const auto* jpeg =
                                    static_cast<const uint8_t*>(cap.in_maps[df.index]);
                                ok = cap.jpeg->decode(std::span<const uint8_t>(jpeg, df.bytesused),
                                                      decoded);
                            }
                        }
                        if (ok) {
                            ++real_frame_idx;
                            broadcast_nv12(prod, decoded, real_frame_idx);
                            last_good_decoded = decoded;
                            // Push the next-broadcast forward so a real
                            // frame's broadcast counts as the tick.
                            next_broadcast = clock::now() + broadcast_period;
                        }
                        if (!cap.cap.queue_buffer(df.index)) {
                            fprintf(stderr,
                                    "videonode-source: QBUF failed (idx=%u errno=%d); "
                                    "kernel ring depth reduced, continuing\n",
                                    df.index, errno);
                        }
                    } else {
                        int e = errno;
                        if (e != ETIMEDOUT && e != EAGAIN) {
                            probe.note_dqbuf_failure(e);
                            if (e == ENODEV) {
                                teardown_session_(cap);
                                last_good_decoded = {};
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
        bool health_changed = (h != prev_health);
        if (health_changed) {
            fprintf(stderr, "videonode-source: state -> %s\n", source_probe::status_text(h));
            prev_health = h;
        }

        // Control-plane status push: on health change, on consumer-count
        // change, or every ~1s as a heartbeat. Drop-on-EAGAIN so a slow
        // daemon can't deadlock the broadcast loop.
        if (ctl_enabled && ctl.connected()) {
            int cur_consumers = prod.consumer_count();
            bool consumers_changed = (cur_consumers != prev_consumer_count);
            bool heartbeat_due = clock::now() >= next_status_heartbeat;
            if (health_changed || consumers_changed || heartbeat_due) {
                std::string params = build_status_params(
                    a.device_id, probe, h, cap, a, real_frame_idx, ph.tick_idx, last_dqbuf_seq,
                    prod);
                ctl.push_status(params);
                prev_consumer_count = cur_consumers;
                next_status_heartbeat = clock::now() + std::chrono::seconds(1);
            }
        }

        // Time to broadcast a tick?
        if (clock::now() < next_broadcast)
            continue;

        if (h == source_probe::Health::Live) {
            // already broadcast via DQBUF path; nothing extra to do here.
            next_broadcast += broadcast_period;
            continue;
        }
        if (h == source_probe::Health::Transitioning && last_good_decoded.fd >= 0) {
            // Re-broadcast last good real frame with fresh sequence.
            ++real_frame_idx;
            broadcast_nv12(prod, last_good_decoded, real_frame_idx);
        } else {
            // Probing / NoCable / NoLock / Gone / Transitioning-without-history.
            nv12_buf::Buffer& ph_buf = ph.paint_and_pick(now_ms(), source_probe::status_text(h));
            broadcast_buffer(prod, ph_buf, ph.tick_idx);
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
        if (!cap.cap.stream_off()) {
            fprintf(stderr, "videonode-source: STREAMOFF failed during shutdown (errno=%d)\n",
                    errno);
        }
        teardown_session_(cap);
    }
    ph.destroy();
    return 0;
}

} // namespace source
