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

#include "control/common.pb.h"
#include "src/capture/source_probe.hpp"
#include "src/render/nv12_buf.hpp"
#include "src/render/placeholder_painter.hpp"
#include "src/rpc/grpc_server.hpp"
#include "src/source/broadcast.hpp"
#include "src/source/capture_session.hpp"
#include "src/source/source_service.hpp"
#include "version.hpp"
#if defined(HAVE_GBM) && !defined(HAVE_RGA)
#include "src/render/csc_gles.hpp"
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
        // stage_ is sized exactly to NV12 of (w, h); the guard inside
        // paint_base can only trip on w/h <= 0 (caller bug). Treat it as
        // an init failure to surface that.
        if (!placeholder_painter::paint_base(stage_, w, h, device_path.c_str()))
            return false;
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
        // stage_ was sized in init() to fit NV12 of (width, height); the
        // false branch can't reach here unless init() also returned false.
        (void)placeholder_painter::paint_tick(stage_, width, height, tick_idx, wallclock_ms,
                                              status);
        nv12_buf::Buffer& b = bufs[idx];
        auto m = nv12_buf::map_rw(b);
        if (m.y) {
            for (int y = 0; y < height; ++y)
                std::memcpy(static_cast<uint8_t*>(m.y) + y * b.y_pitch, stage_.data() + y * width,
                            width);
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
           "  --grpc-listen PATH            per-instance UDS where the source's gRPC server\n"
           "                                  binds (the daemon dials in). Omit for standalone.\n"
           "  --device-id ID                stable device ID advertised via Source.Describe()\n",
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
        } else if (s == "--device-id") {
            const char* v = eat(i);
            if (!v)
                return false;
            a.device_id = v;
        } else if (s == "--grpc-listen") {
            const char* v = eat(i);
            if (!v)
                return false;
            a.grpc_listen = v;
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
    // no RGA) the GBM allocator MUST share csc_gles's gbm_device —
    // radeonsi rejects cross-gbm_device dma-buf imports as renderbuffer
    // storage, so allocating against a sibling device produces FBO-
    // incomplete and no frames flow. csc_gles::init() lazy-creates its
    // EGL+GBM stack on first call; we force-init eagerly here so the
    // allocator has the right device.
#if defined(HAVE_GBM) && !defined(HAVE_RGA)
    if (!csc_gles::init()) {
        fprintf(stderr, "videonode-source: csc_gles::init failed; cannot bring up Mesa CSC backend "
                        "(needed for the GBM allocator's gbm_device)\n");
        return 1;
    }
    gbm_device* alloc_gbm = csc_gles::gbm_device_for_io();
    if (alloc_gbm == nullptr) {
        fprintf(stderr, "videonode-source: csc_gles::gbm_device_for_io returned null\n");
        return 1;
    }
    nv12_buf::Allocator allocator;
    if (!allocator.init(alloc_gbm)) {
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

    // Control plane: --grpc-listen + --device-id together bring up an
    // in-process gRPC server (nativerpc::SourceService) that the daemon
    // dials. Both empty → standalone mode (R smoke scenarios), no
    // server, no daemon-issued SetFormat / Snapshot / status stream.
    bool need_reinit_for_format_change = false;

    nativerpc::SourceContext gctx;
    gctx.device_id = a.device_id;
    gctx.version = vn::kVersion;
    gctx.running = &running;
    gctx.args = &a;
    gctx.need_reinit_for_format_change = &need_reinit_for_format_change;
    gctx.probe = &probe;
    nativerpc::SourceService grpc_svc(&gctx);
    nativerpc::GrpcServer grpc_srv;
    bool grpc_enabled = !a.grpc_listen.empty() && !a.device_id.empty();
    if (grpc_enabled) {
        std::vector<grpc::Service*> services = {&grpc_svc};
        if (!grpc_srv.Start(a.grpc_listen, services)) {
            fprintf(stderr, "videonode-source: gRPC server failed to start on %s\n",
                    a.grpc_listen.c_str());
            grpc_enabled = false;
        } else {
            fprintf(stderr, "videonode-source: grpc server listening on %s (id=%s)\n",
                    a.grpc_listen.c_str(), a.device_id.c_str());
        }
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

        // Prune dead consumers on every loop iteration. broadcast()'s
        // in-band eviction stalls during DQBUF gaps (signal transitions) or
        // when next_broadcast keeps being pushed forward by a steady DQBUF
        // stream; this keeps the consumer list bounded regardless.
        (void)prod.prune_dead_consumers();

        // Format-change reinit: synchronous teardown + reopen with the
        // new args. The gRPC SetFormat handler runs on a separate thread
        // and mutates `a` + `need_reinit_for_format_change` under
        // set_format_mu; copy out the flag (and atomically clear it)
        // under the lock so the reinit reads a consistent Args snapshot.
        // try_open_capture takes Args by const ref so further writes by
        // SetFormat during the V4L2 ioctls only affect the *next* loop
        // iteration's reinit.
        bool reinit_now = false;
        {
            std::lock_guard<std::mutex> lock(gctx.set_format_mu);
            if (need_reinit_for_format_change) {
                reinit_now = true;
                need_reinit_for_format_change = false;
            }
        }
        if (reinit_now) {
            last_good_decoded = {};
            // Snapshot Args under the lock so try_open_capture sees a
            // coherent set even if SetFormat races us.
            Args snap;
            {
                std::lock_guard<std::mutex> lock(gctx.set_format_mu);
                snap = a;
            }
            if (try_open_capture(cap, snap, allocator)) {
                probe.attach();
                need_reinit = false;
            } else {
                need_reinit = true;
            }
        }

        // Reinit capture if we lost it.
        if (need_reinit) {
            Args snap;
            {
                std::lock_guard<std::mutex> lock(gctx.set_format_mu);
                snap = a;
            }
            if (try_open_capture(cap, snap, allocator)) {
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

        // Build pollset: capture fd (if active). The gRPC control plane
        // runs on its own thread (see nativerpc::GrpcServer), so we no
        // longer multiplex its socket through this poll.
        std::vector<pollfd> pset;
        int cap_idx = -1;
        if (cap.active) {
            pollfd pfd{};
            pfd.fd = cap.cap.fd();
            pfd.events = POLLIN | POLLPRI;
            cap_idx = int(pset.size());
            pset.push_back(pfd);
        }

        if (!pset.empty()) {
            int pr = ::poll(pset.data(), pset.size(), poll_timeout_ms);
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
                            // Split-allocator (host GBM) gives distinct
                            // y_fd / uv_fd; single-buffer (rig dma_heap)
                            // shares one fd at different offsets. csc_gles
                            // distinguishes via uv_fd ≥ 0.
                            dst_p.uv_fd = (dst_buf.uv_fd != dst_buf.y_fd) ? dst_buf.uv_fd : -1;
                            dst_p.uv_wstride = int(dst_buf.uv_pitch);
                            dst_p.fmt = csc::PixelFormat::Nv12;
                            dst_p.width = cap.width;
                            dst_p.height = cap.height;
                            dst_p.wstride = int(dst_buf.y_pitch);
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
                            if (grpc_enabled) {
                                nativerpc::LatestFrame lf;
                                if (snapshot_nv12_from_decoded(decoded, real_frame_idx, lf)) {
                                    grpc_svc.UpdateLastFrame(std::move(lf));
                                }
                            }
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

        // Control-plane status push (gRPC StreamStatus subscribers): on
        // health change, consumer-count change, or every ~1s as a
        // heartbeat. PublishStatus is non-blocking — slow subscribers
        // see stale snapshots, not deadlock.
        if (grpc_enabled) {
            int cur_consumers = prod.consumer_count();
            bool consumers_changed = (cur_consumers != prev_consumer_count);
            bool heartbeat_due = clock::now() >= next_status_heartbeat;
            if (health_changed || consumers_changed || heartbeat_due) {
                videonode::control::Status sp;
                build_status_proto(sp, a.device_id, probe, h, cap, a, real_frame_idx, ph.tick_idx,
                                   last_dqbuf_seq, prod);
                grpc_svc.PublishStatus(sp);
                prev_consumer_count = cur_consumers;
                next_status_heartbeat = clock::now() + std::chrono::seconds(1);
            }
        }

        // Time to broadcast a tick?
        if (clock::now() < next_broadcast)
            continue;

        if (h == source_probe::Health::Live) {
            // already broadcast via DQBUF path; nothing extra to do here.
            // (prune_dead_consumers runs unconditionally at the top of every
            // loop iteration, so dead consumers are reaped regardless of the
            // broadcast cadence.)
            next_broadcast += broadcast_period;
            continue;
        }
        if (h == source_probe::Health::Transitioning && last_good_decoded.fd >= 0) {
            // Re-broadcast last good real frame with fresh sequence.
            ++real_frame_idx;
            broadcast_nv12(prod, last_good_decoded, real_frame_idx);
            if (grpc_enabled) {
                nativerpc::LatestFrame lf;
                if (snapshot_nv12_from_decoded(last_good_decoded, real_frame_idx, lf)) {
                    grpc_svc.UpdateLastFrame(std::move(lf));
                }
            }
        } else {
            // Probing / NoCable / NoLock / Gone / Transitioning-without-history.
            nv12_buf::Buffer& ph_buf = ph.paint_and_pick(now_ms(), source_probe::status_text(h));
            broadcast_buffer(prod, ph_buf, ph.tick_idx);
            if (grpc_enabled) {
                nativerpc::LatestFrame lf;
                if (snapshot_nv12_from_buffer(ph_buf, ph.tick_idx, lf)) {
                    grpc_svc.UpdateLastFrame(std::move(lf));
                }
            }
        }
        next_broadcast += broadcast_period;
        if (next_broadcast < clock::now()) {
            next_broadcast = clock::now() + broadcast_period;
        }
    }

    fprintf(stderr, "videonode-source: shutting down (real=%llu placeholder=%llu)\n",
            static_cast<unsigned long long>(real_frame_idx),
            static_cast<unsigned long long>(ph.tick_idx));
    if (grpc_enabled) {
        grpc_svc.StopStreams();
        grpc_srv.Shutdown();
    }
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
