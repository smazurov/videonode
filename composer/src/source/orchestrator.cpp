// SourceOrchestrator implementation. See orchestrator.hpp.
//
// Main loop is event-driven: poll(fd, POLLIN|POLLPRI) wakes on a ready
// frame or a V4L2 event. SourceProbe consumes those plus DQBUF results
// to compute a health state (Live / Transitioning / NoCable / NoLock /
// Gone / Probing). The broadcaster ticks at a fixed rate:
//
//   Live          -> newest real-frame fd (RGA-CSC NV12, or MPP/TurboJPEG MJPEG decode)
//   Transitioning -> last-good real-frame fd, re-broadcast (no flicker)
//   else          -> painted placeholder with status text
//
// No time-based "stale" thresholds: state comes from V4L2 events + ctrls.

#include "src/source/orchestrator.hpp"

#include "control/common.pb.h"
#include "src/capture/source_probe.hpp"
#include "src/common/log_levels.hpp"
#include "src/render/nv12_buf.hpp"
#include "src/render/placeholder_painter.hpp"
#include "src/rpc/grpc_server.hpp"
#include "src/snapshot/snapshot.hpp"
#include "src/source/broadcast.hpp"
#include "src/source/capture_session.hpp"
#include "src/source/source_service.hpp"
#include "version.hpp"
#if defined(HAVE_GBM) && !defined(HAVE_RGA)
#include "src/render/csc_placebo.hpp"
#endif

#include <atomic>
#include <cerrno>
#include <chrono>
#include <cstring>
#include <linux/videodev2.h>
#include <poll.h>
#include <span>
#include <string>
#include <thread>
#include <unistd.h>
#include <vector>

namespace source {

namespace {

uint64_t monotonic_ns_() {
    using namespace std::chrono;
    return static_cast<uint64_t>(
        duration_cast<nanoseconds>(steady_clock::now().time_since_epoch()).count());
}

// Build a FrameRef for the source's NV12 holder. Plane[0] = Y, plane[1] =
// UV. UV may reuse the Y fd (single-allocation dma_heap on rig) or have a
// distinct fd (split GBM allocation on host); the source's DecodedNv12
// already carries both shapes via plane1_fd.
vn::snapshot::FrameRef make_frame_ref_(const jpeg_dec::DecodedNv12& d, uint64_t frame_idx) {
    vn::snapshot::FrameRef r{};
    r.format = vn::snapshot::Format::Nv12;
    r.width = static_cast<uint32_t>(d.width);
    r.height = static_cast<uint32_t>(d.height);
    r.pitch_y = d.y_pitch;
    r.pitch_uv = d.uv_pitch;
    r.planes[0] = {.fd = d.fd,
                   .offset = d.y_offset,
                   .pitch = d.y_pitch,
                   .row_bytes = static_cast<size_t>(d.width),
                   .rows = static_cast<size_t>(d.height)};
    const int uv_fd = d.plane1_fd >= 0 ? d.plane1_fd : d.fd;
    r.planes[1] = {.fd = uv_fd,
                   .offset = d.uv_offset,
                   .pitch = d.uv_pitch,
                   .row_bytes = static_cast<size_t>(d.width),
                   .rows = static_cast<size_t>(d.height) / 2};
    r.frame_idx = frame_idx;
    r.captured_at_ns = monotonic_ns_();
    return r;
}

vn::snapshot::FrameRef make_frame_ref_(const nv12_buf::Buffer& b, uint64_t frame_idx) {
    jpeg_dec::DecodedNv12 d;
    d.fd = b.y_fd;
    d.plane1_fd = b.uv_fd;
    d.width = b.width;
    d.height = b.height;
    d.y_pitch = b.y_pitch;
    d.uv_pitch = b.uv_pitch;
    d.y_offset = b.y_offset;
    d.uv_offset = b.uv_offset;
    return make_frame_ref_(d, frame_idx);
}

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

int Run(const Args& a_in, std::atomic<bool>& running) {
    // Local mutable copy: set_format requests at runtime mutate in_format/
    // width/height/fps for the reinit-with-new-args path below.
    Args a = a_in;

    // NV12 output allocator. On rig (HAVE_RGA) this is stateless and
    // backed by dma_heap (single bo); on Fedora / Mesa hosts (HAVE_GBM,
    // no RGA) the GBM allocator MUST share csc_gles's gbm_device —
    // radeonsi rejects cross-gbm_device dma-buf imports as renderbuffer
    // storage, so allocating against a sibling device produces FBO-
    // incomplete and no frames flow. csc_placebo::init() lazy-creates its
    // EGL+GBM stack on first call; we force-init eagerly here so the
    // allocator has the right device.
#if defined(HAVE_GBM) && !defined(HAVE_RGA)
    if (!csc_placebo::init()) {
        vn::log::fatal(
            "videonode-source: csc_placebo::init failed; cannot bring up Mesa CSC backend "
            "(needed for the GBM allocator's gbm_device)");
        return 1;
    }
    gbm_device* alloc_gbm = csc_placebo::gbm_device_for_io();
    if (alloc_gbm == nullptr) {
        vn::log::fatal("videonode-source: csc_placebo::gbm_device_for_io returned null");
        return 1;
    }
    nv12_buf::Allocator allocator;
    if (!allocator.init(alloc_gbm)) {
        vn::log::fatal("videonode-source: nv12_buf::Allocator::init failed");
        return 1;
    }
#else
    nv12_buf::Allocator allocator;
    if (!allocator.init()) {
        vn::log::fatal("videonode-source: nv12_buf::Allocator::init failed");
        return 1;
    }
#endif

    PlaceholderRing ph;
    if (!ph.init(allocator, a.placeholder_w, a.placeholder_h, a.device)) {
        vn::log::fatal("videonode-source: failed to allocate placeholder ring");
        return 1;
    }
    vn::log::info("videonode-source: placeholder %dx%d ready", a.placeholder_w, a.placeholder_h);

    scm_rights_producer::ScmRightsProducer prod;
    scm_rights_producer::InitParams pp;
    pp.socket_path = a.out_socket;
    pp.max_consumers = a.max_consumers;
    if (!prod.init(pp) || !prod.start())
        return 1;

    CaptureSession cap;
    source_probe::SourceProbe probe(cap.cap);

    // Control plane: --grpc-listen + --device-id together bring up an
    // in-process gRPC server (nativerpc::SourceService) that the daemon
    // dials. Both empty → standalone mode (R smoke scenarios), no
    // server, no daemon-issued SetFormat / Snapshot / status stream.
    bool need_reinit_for_format_change = false;
    // active_format mirrors the post-negotiation format whenever cap is
    // streaming. Populated under gctx.set_format_mu after every successful
    // try_open_capture; cleared on teardown. SourceService::SetFormat
    // reads it to skip rebuilds when the request already matches.
    std::optional<nativerpc::ActiveFormat> active_format;

    nativerpc::SourceContext gctx;
    gctx.device_id = a.device_id;
    gctx.version = vn::kVersion;
    gctx.running = &running;
    gctx.args = &a;
    gctx.need_reinit_for_format_change = &need_reinit_for_format_change;
    gctx.probe = &probe;
    gctx.active_format = &active_format;

    auto publish_active_format = [&](const Args& used) {
        std::lock_guard<std::mutex> lock(gctx.set_format_mu);
        active_format = nativerpc::ActiveFormat{.fourcc = cap.src_fmt_name,
                                                .w = static_cast<uint32_t>(cap.width),
                                                .h = static_cast<uint32_t>(cap.height),
                                                .fps = static_cast<uint32_t>(used.in_fps)};
    };
    auto clear_active_format = [&]() {
        std::lock_guard<std::mutex> lock(gctx.set_format_mu);
        active_format.reset();
    };

    if (a.device.empty()) {
        // Daemon-managed sources start with no device; SetDevice arrives
        // later. Skip the initial open() entirely to avoid logging
        // open(""): ENOENT before the daemon has assigned a path.
    } else if (try_open_capture(cap, a, allocator)) {
        probe.attach();
        publish_active_format(a);
    } else {
        vn::log::warn("videonode-source: capture not ready at startup");
    }
    nativerpc::SourceService grpc_svc(&gctx);
    nativerpc::GrpcServer grpc_srv;
    bool grpc_enabled = !a.grpc_listen.empty() && !a.device_id.empty();
    if (grpc_enabled) {
        std::vector<grpc::Service*> services = {&grpc_svc};
        if (!grpc_srv.Start(a.grpc_listen, services)) {
            vn::log::fatal("videonode-source: gRPC server failed to start on %s",
                           a.grpc_listen.c_str());
            grpc_enabled = false;
        } else {
            vn::log::info("videonode-source: grpc server listening on %s (id=%s)",
                          a.grpc_listen.c_str(), a.device_id.c_str());
        }
    }

    using clock = std::chrono::steady_clock;
    const auto broadcast_period =
        std::chrono::nanoseconds(1'000'000'000LL / std::max(1, a.placeholder_broadcast_fps));
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
            if (snap.device.empty()) {
                // SetDevice("") detached us; tear down any open capture
                // and stay in placeholder mode until a new path arrives.
                teardown_session_(cap);
                clear_active_format();
                need_reinit = false;
            } else if (try_open_capture(cap, snap, allocator)) {
                probe.attach();
                publish_active_format(snap);
                need_reinit = false;
            } else {
                clear_active_format();
                need_reinit = true;
            }
        }

        // Reinit capture if we lost it. Empty device means daemon hasn't
        // assigned one yet (or detached us); skip silently so the loop
        // doesn't spin on open() retries.
        if (need_reinit) {
            Args snap;
            {
                std::lock_guard<std::mutex> lock(gctx.set_format_mu);
                snap = a;
            }
            if (snap.device.empty()) {
                need_reinit = false;
            } else if (try_open_capture(cap, snap, allocator)) {
                probe.attach();
                publish_active_format(snap);
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
                                clear_active_format();
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
                                nv12_buf::stage_for_read(dst_buf);
                                decoded.fd = (dst_buf.staged_y_fd >= 0) ? dst_buf.staged_y_fd
                                                                        : dst_buf.y_fd;
                                decoded.plane1_fd = (dst_buf.staged_uv_fd >= 0)
                                                        ? dst_buf.staged_uv_fd
                                                        : dst_buf.uv_fd;
                                decoded.width = cap.width;
                                decoded.height = cap.height;
                                decoded.y_pitch = dst_buf.y_pitch;
                                decoded.uv_pitch = dst_buf.uv_pitch;
                                decoded.y_offset =
                                    (dst_buf.staged_y_fd >= 0) ? 0 : dst_buf.y_offset;
                                decoded.uv_offset =
                                    (dst_buf.staged_uv_fd >= 0) ? 0 : dst_buf.uv_offset;
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
                                grpc_svc.UpdateLastFrame(make_frame_ref_(decoded, real_frame_idx));
                            }
                            last_good_decoded = decoded;
                            // Push the next-broadcast forward so a real
                            // frame's broadcast counts as the tick.
                            next_broadcast = clock::now() + broadcast_period;
                        }
                        if (!cap.cap.queue_buffer(df.index)) {
                            vn::log::warn("videonode-source: QBUF failed (idx=%u errno=%d); "
                                          "kernel ring depth reduced, continuing",
                                          df.index, errno);
                        }
                    } else {
                        int e = errno;
                        if (e != ETIMEDOUT && e != EAGAIN) {
                            probe.note_dqbuf_failure(e);
                            if (e == ENODEV) {
                                teardown_session_(cap);
                                clear_active_format();
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
            vn::log::info("videonode-source: state -> %s", source_probe::status_text(h));
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
                grpc_svc.UpdateLastFrame(make_frame_ref_(last_good_decoded, real_frame_idx));
            }
        } else {
            // Probing / NoCable / NoLock / Gone / Transitioning-without-history.
            nv12_buf::Buffer& ph_buf = ph.paint_and_pick(now_ms(), source_probe::status_text(h));
            nv12_buf::stage_for_read(ph_buf);
            broadcast_buffer(prod, ph_buf, ph.tick_idx);
            if (grpc_enabled) {
                grpc_svc.UpdateLastFrame(make_frame_ref_(ph_buf, ph.tick_idx));
            }
        }
        next_broadcast += broadcast_period;
        if (next_broadcast < clock::now()) {
            next_broadcast = clock::now() + broadcast_period;
        }
    }

    vn::log::info("videonode-source: shutting down (real=%llu placeholder=%llu)",
                  static_cast<unsigned long long>(real_frame_idx),
                  static_cast<unsigned long long>(ph.tick_idx));
    if (grpc_enabled) {
        grpc_svc.StopStreams();
        grpc_srv.Shutdown();
    }
    prod.stop();
    if (cap.active) {
        if (!cap.cap.stream_off()) {
            vn::log::error("videonode-source: STREAMOFF failed during shutdown (errno=%d)", errno);
        }
        teardown_session_(cap);
    }
    ph.destroy();
    return 0;
}

} // namespace source
