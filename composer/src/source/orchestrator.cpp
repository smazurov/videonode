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
#include "src/common/log_keys.hpp"
#include "src/common/log_levels.hpp"
#include "src/render/nv12_buf.hpp"
#include "src/render/placeholder_painter.hpp"
#include "src/rpc/grpc_server.hpp"
#include "src/snapshot/snapshot.hpp"
#include "src/source/broadcast.hpp"
#include "src/source/capture_poll.hpp"
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
#include <optional>
#include <poll.h>
#include <span>
#include <string>
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
        std::span<const uint8_t> stage_span(stage_);
        auto y_plane = stage_span.first(size_t(w) * h);
        auto uv_plane = stage_span.subspan(size_t(w) * h);
        for (int i = 0; i < 2; ++i) {
            nv12_buf::Buffer b = alloc.alloc(w, h);
            if (!b.valid())
                return false;
            auto m = nv12_buf::map_rw(b);
            if (!m.y || !m.uv)
                return false;
            auto dst_y = m.y_bytes();
            auto dst_uv = m.uv_bytes();
            for (int y = 0; y < h; ++y) {
                std::memcpy(dst_y.subspan(size_t(y) * b.y_pitch, size_t(w)).data(),
                            y_plane.subspan(size_t(y) * w, size_t(w)).data(), size_t(w));
            }
            for (int y = 0; y < h / 2; ++y) {
                std::memcpy(dst_uv.subspan(size_t(y) * b.uv_pitch, size_t(w)).data(),
                            uv_plane.subspan(size_t(y) * w, size_t(w)).data(), size_t(w));
            }
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
            std::span<const uint8_t> stage_span(stage_);
            auto dst_y = m.y_bytes();
            for (int y = 0; y < height; ++y) {
                std::memcpy(dst_y.subspan(size_t(y) * b.y_pitch, size_t(width)).data(),
                            stage_span.subspan(size_t(y) * width, size_t(width)).data(),
                            size_t(width));
            }
        }
        nv12_buf::unmap(b);
        return b;
    }
    void destroy() {
        bufs.clear();
        stage_.clear();
    }
};

// LoopState bundles mutable state that is shared across the helper
// functions extracted from Run.
struct LoopState {
    using clock = std::chrono::steady_clock;

    PlaceholderRing& ph;
    scm_rights_producer::ScmRightsProducer& prod;
    CaptureSession& cap;
    source_probe::SourceProbe& probe;
    nativerpc::SourceService& grpc_svc;
    nativerpc::SourceContext& gctx;
    Args& a;
    bool grpc_enabled;

    uint64_t real_frame_idx = 0;
    uint32_t last_dqbuf_seq = 0;
    jpeg_dec::DecodedNv12 last_good_decoded{};
    source_probe::Health prev_health = source_probe::Health::Probing;
    bool need_reinit = false;
    clock::time_point next_broadcast;
    clock::time_point next_power_poll;
    clock::time_point next_status_heartbeat;
    clock::time_point next_reinit_attempt;                      // 1 Hz reopen pacing
    CaptureOpenStatus prev_open_status = CaptureOpenStatus::Ok; // gate Failed log to transition
    int prev_consumer_count = -1;
    std::chrono::nanoseconds broadcast_period;
};

// Handle V4L2 priority events (SOURCE_CHANGE).
void handle_v4l2_events_(LoopState& st) {
    std::vector<v4l2_event> evs;
    if (!st.cap.cap.drain_events_typed(evs))
        return;
    bool need_restart = false;
    for (const auto& e : evs) {
        st.probe.note_event(e);
        if (e.type == V4L2_EVENT_SOURCE_CHANGE)
            need_restart = true;
    }
    if (!need_restart)
        return;
    if (st.cap.cap.restart_streaming()) {
        st.probe.note_streaming_restarted();
    } else {
        teardown_session_(st.cap);
        st.need_reinit = true;
    }
}

// Dequeue one frame, CSC/decode it, and broadcast it. Returns true if a
// real frame was successfully broadcast.
bool handle_dqbuf_(LoopState& st) {
    v4l2::DequeuedFrame df;
    if (!st.cap.cap.dequeue_buffer(0, df)) {
        int e = errno;
        if (e != ETIMEDOUT && e != EAGAIN) {
            st.probe.note_dqbuf_failure(e);
            if (e == ENODEV) {
                teardown_session_(st.cap);
                st.last_good_decoded = {};
                st.need_reinit = true;
            }
        }
        return false;
    }

    st.probe.note_dqbuf_success();
    st.last_dqbuf_seq = df.sequence;

    bool ok = false;
    jpeg_dec::DecodedNv12 decoded;
    if (st.cap.mode == DecodeMode::Rga) {
        uint32_t ring_idx = st.cap.out_ring_write;
        nv12_buf::Buffer& dst_buf = st.cap.out_ring[ring_idx];
        csc::ConvertParams src_p, dst_p;
        src_p.fd = st.cap.cap.buffers()[df.index].primary_dma_buf();
        src_p.fmt = st.cap.src_fmt;
        src_p.width = st.cap.width;
        src_p.height = st.cap.height;
        dst_p.fd = dst_buf.y_fd;
        dst_p.uv_fd = (dst_buf.uv_fd != dst_buf.y_fd) ? dst_buf.uv_fd : -1;
        dst_p.uv_wstride = int(dst_buf.uv_pitch);
        dst_p.fmt = csc::PixelFormat::Nv12;
        dst_p.width = st.cap.width;
        dst_p.height = st.cap.height;
        dst_p.wstride = int(dst_buf.y_pitch);
        if (csc::convert(src_p, dst_p)) {
            st.cap.out_ring_write = (ring_idx + 1) % static_cast<uint32_t>(st.cap.out_ring.size());
            nv12_buf::stage_for_read(dst_buf);
            decoded.fd = (dst_buf.staged_y_fd >= 0) ? dst_buf.staged_y_fd : dst_buf.y_fd;
            decoded.plane1_fd = (dst_buf.staged_uv_fd >= 0) ? dst_buf.staged_uv_fd : dst_buf.uv_fd;
            decoded.width = st.cap.width;
            decoded.height = st.cap.height;
            decoded.y_pitch = dst_buf.y_pitch;
            decoded.uv_pitch = dst_buf.uv_pitch;
            decoded.y_offset = (dst_buf.staged_y_fd >= 0) ? 0 : dst_buf.y_offset;
            decoded.uv_offset = (dst_buf.staged_uv_fd >= 0) ? 0 : dst_buf.uv_offset;
            ok = true;
        }
    } else { // DecodeMode::Mjpeg
        if (df.index < st.cap.in_maps.size() && df.bytesused > 0) {
            const auto* jpeg = static_cast<const uint8_t*>(st.cap.in_maps[df.index]);
            ok = st.cap.jpeg->decode(std::span<const uint8_t>(jpeg, df.bytesused), decoded);
        }
    }

    if (ok) {
        ++st.real_frame_idx;
        broadcast_nv12(st.prod, decoded, st.real_frame_idx, to_header_matrix(st.cap.color_matrix));
        if (st.grpc_enabled)
            st.grpc_svc.UpdateLastFrame(make_frame_ref(decoded, st.real_frame_idx));
        st.last_good_decoded = decoded;
        st.next_broadcast = LoopState::clock::now() + st.broadcast_period;
    }

    if (!st.cap.cap.queue_buffer(df.index)) {
        vn::log::warn("videonode-source: QBUF failed (idx=%u errno=%d); "
                      "kernel ring depth reduced, continuing",
                      df.index, errno);
    }

    return ok;
}

// Poll the capture fd with a deadline, dispatch V4L2 events + DQBUF.
void poll_and_dispatch_(LoopState& st) {
    auto until_next = st.next_broadcast - LoopState::clock::now();
    int poll_timeout_ms =
        int(std::chrono::duration_cast<std::chrono::milliseconds>(until_next).count());
    if (poll_timeout_ms < 0)
        poll_timeout_ms = 0;
    if (poll_timeout_ms > 100)
        poll_timeout_ms = 100;

    std::vector<pollfd> pset;
    int cap_idx = -1;
    if (st.cap.active) {
        pollfd pfd{};
        pfd.fd = st.cap.cap.fd();
        pfd.events = POLLIN | POLLPRI;
        cap_idx = int(pset.size());
        pset.push_back(pfd);
    }

    if (pset.empty()) {
        std::this_thread::sleep_until(st.next_broadcast);
        return;
    }

    int pr = ::poll(pset.data(), pset.size(), poll_timeout_ms);
    if (pr <= 0 || cap_idx < 0)
        return;

    pollfd pfd = pset[cap_idx];
    CapturePollAction act = classify_capture_poll(pfd.revents);
    if (act.error) {
        // A USB capture stall under load latches POLLERR/POLLHUP on the fd.
        // The dispatch below only reacts to POLLPRI/POLLIN, so left unhandled
        // poll() returns immediately every iteration -> the loop pins a core
        // at 100% and never recovers. Note the failure (health -> NoLock),
        // drop the faulted session so the existing reinit path reopens a clean
        // fd next tick, and pace to the broadcast deadline so repeated
        // open-then-fault cycles run at frame rate instead of busy-spinning.
        vn::log::warn("videonode-source: capture fd error revents=0x%x, reinit",
                      static_cast<unsigned>(pfd.revents));
        st.probe.note_dqbuf_failure(EIO);
        teardown_session_(st.cap);
        st.last_good_decoded = {};
        st.need_reinit = true;
        auto now = LoopState::clock::now();
        if (st.next_broadcast <= now)
            st.next_broadcast = now + st.broadcast_period;
        std::this_thread::sleep_until(st.next_broadcast);
        return;
    }
    if (act.drain_events)
        handle_v4l2_events_(st);
    if (act.dequeue)
        handle_dqbuf_(st);
}

// Push a status proto to gRPC subscribers when warranted.
void maybe_publish_status_(LoopState& st, source_probe::Health h, bool health_changed,
                           uint64_t placeholder_frames) {
    if (!st.grpc_enabled)
        return;
    int cur_consumers = st.prod.consumer_count();
    bool consumers_changed = (cur_consumers != st.prev_consumer_count);
    bool heartbeat_due = LoopState::clock::now() >= st.next_status_heartbeat;
    if (!health_changed && !consumers_changed && !heartbeat_due)
        return;
    videonode::control::Status sp;
    StatusContext ctx{.device_id = st.a.device_id,
                      .probe = st.probe,
                      .health = h,
                      .cap = st.cap,
                      .args = st.a,
                      .real_frame_idx = st.real_frame_idx,
                      .placeholder_frames = placeholder_frames,
                      .last_seq = st.last_dqbuf_seq,
                      .prod = st.prod};
    build_status_proto(sp, ctx);
    st.grpc_svc.PublishStatus(sp);
    st.prev_consumer_count = cur_consumers;
    st.next_status_heartbeat = LoopState::clock::now() + std::chrono::seconds(1);
}

// Broadcast a tick (placeholder or re-broadcast of last good frame).
void broadcast_tick_(LoopState& st, source_probe::Health h) {
    if (h == source_probe::Health::Transitioning && st.last_good_decoded.fd >= 0) {
        ++st.real_frame_idx;
        broadcast_nv12(st.prod, st.last_good_decoded, st.real_frame_idx,
                       to_header_matrix(st.cap.color_matrix));
        if (st.grpc_enabled)
            st.grpc_svc.UpdateLastFrame(make_frame_ref(st.last_good_decoded, st.real_frame_idx));
    } else {
        nv12_buf::Buffer& ph_buf = st.ph.paint_and_pick(now_ms(), source_probe::status_text(h));
        nv12_buf::stage_for_read(ph_buf);
        broadcast_buffer(st.prod, ph_buf, st.ph.tick_idx);
        if (st.grpc_enabled)
            st.grpc_svc.UpdateLastFrame(make_frame_ref(ph_buf, st.ph.tick_idx));
    }
}

// Attempt (re)open of the capture device, publish active_format on success.
// Paced to 1 Hz so an absent device doesn't busy-spin open(); the errno
// classification drives the probe's health (no_signal vs initializing).
void maybe_reinit_capture_(LoopState& st, nv12_buf::Allocator& allocator,
                           auto& publish_active_format) {
    if (!st.need_reinit)
        return;
    auto now = LoopState::clock::now();
    if (now < st.next_reinit_attempt)
        return;
    Args snap;
    {
        std::lock_guard<std::mutex> lock(st.gctx.set_format_mu);
        snap = st.a;
    }
    if (snap.device.empty()) {
        st.need_reinit = false;
        return;
    }
    CaptureOpenStatus status = try_open_capture(st.cap, snap, allocator, /*quiet=*/true);
    int open_errno = errno; // best-effort; meaningful for open-stage failures
    st.next_reinit_attempt = now + std::chrono::seconds(1);
    switch (status) {
    case CaptureOpenStatus::Ok:
        st.probe.attach();
        publish_active_format(snap);
        st.need_reinit = false;
        break;
    case CaptureOpenStatus::Busy:
        // Node present, udev still settling perms — report the bring-up state
        // (initializing) and keep retrying quietly.
        st.probe.note_device_acquiring();
        vn::log::debug("videonode-source: %s present but busy, retrying", snap.device.c_str());
        break;
    case CaptureOpenStatus::Absent:
        // Expected on unplug. The Live -> NO SIGNAL health-change line is the
        // single user-facing log; stay quiet per-attempt.
        st.probe.note_device_absent();
        break;
    case CaptureOpenStatus::Failed:
        st.probe.note_device_absent();
        if (st.prev_open_status != CaptureOpenStatus::Failed)
            vn::log::error("videonode-source: open(%s) failed (errno=%d)", snap.device.c_str(),
                           open_errno);
        break;
    }
    st.prev_open_status = status;
    // even if reinit failed we proceed — placeholder still ticks
}

// Handle a format-change request from the gRPC SetFormat handler.
void handle_format_change_(LoopState& st, nv12_buf::Allocator& allocator,
                           auto& publish_active_format, auto& clear_active_format) {
    bool reinit_now = false;
    {
        std::lock_guard<std::mutex> lock(st.gctx.set_format_mu);
        if (*st.gctx.need_reinit_for_format_change) {
            reinit_now = true;
            *st.gctx.need_reinit_for_format_change = false;
        }
    }
    if (!reinit_now)
        return;

    st.last_good_decoded = {};
    Args snap;
    {
        std::lock_guard<std::mutex> lock(st.gctx.set_format_mu);
        snap = st.a;
    }
    if (snap.device.empty()) {
        teardown_session_(st.cap);
        clear_active_format();
        st.need_reinit = false;
    } else if (try_open_capture(st.cap, snap, allocator) == CaptureOpenStatus::Ok) {
        st.probe.attach();
        publish_active_format(snap);
        st.need_reinit = false;
    } else {
        clear_active_format();
        st.need_reinit = true;
    }
}

// Initialize the NV12 allocator. Returns false and logs on failure.
// On rig (HAVE_RGA) this is dma_heap-backed; on host (HAVE_GBM, no RGA)
// the allocator must share csc_placebo's GBM device.
bool init_allocator_(nv12_buf::Allocator& allocator) {
#if defined(HAVE_GBM) && !defined(HAVE_RGA)
    if (!csc_placebo::init()) {
        vn::log::fatal(
            "videonode-source: csc_placebo::init failed; cannot bring up Mesa CSC backend "
            "(needed for the GBM allocator's gbm_device)");
        return false;
    }
    gbm_device* alloc_gbm = csc_placebo::gbm_device_for_io();
    if (alloc_gbm == nullptr) {
        vn::log::fatal("videonode-source: csc_placebo::gbm_device_for_io returned null");
        return false;
    }
    if (!allocator.init(alloc_gbm)) {
        vn::log::fatal("videonode-source: nv12_buf::Allocator::init failed");
        return false;
    }
#else
    if (!allocator.init()) {
        vn::log::fatal("videonode-source: nv12_buf::Allocator::init failed");
        return false;
    }
#endif
    return true;
}

// Start the gRPC server if configured. Returns false on failure (grpc_enabled
// is set to false in that case so the caller can branch on it).
bool start_grpc_(const Args& a, nativerpc::SourceService& grpc_svc, nativerpc::GrpcServer& grpc_srv,
                 bool& grpc_enabled) {
    grpc_enabled = !a.grpc_listen.empty() && !a.device_id.empty();
    if (!grpc_enabled)
        return true;
    std::vector<grpc::Service*> services = {&grpc_svc};
    if (!grpc_srv.Start(a.grpc_listen, services)) {
        vn::log::fatal("videonode-source: gRPC server failed to start on %s",
                       a.grpc_listen.c_str());
        grpc_enabled = false;
        return false;
    }
    vn::log::debug("videonode-source: grpc server listening on %s (id=%s)", a.grpc_listen.c_str(),
                   a.device_id.c_str());
    return true;
}

// Populate a SourceContext from the objects Run owns.
void populate_gctx_(nativerpc::SourceContext& gctx, std::atomic<bool>& running, Args& a,
                    bool& need_reinit_flag, source_probe::SourceProbe& probe,
                    std::optional<nativerpc::ActiveFormat>& active_format) {
    gctx.device_id = a.device_id;
    gctx.version = vn::kVersion;
    gctx.running = &running;
    gctx.args = &a;
    gctx.need_reinit_for_format_change = &need_reinit_flag;
    gctx.probe = &probe;
    gctx.active_format = &active_format;
}

// Shut down gRPC, prod, cap, and the placeholder ring.
void shutdown_(LoopState& st, nativerpc::GrpcServer& grpc_srv, PlaceholderRing& ph) {
    char real_s[24], ph_s[24];
    std::snprintf(real_s, sizeof(real_s), "%llu",
                  static_cast<unsigned long long>(st.real_frame_idx));
    std::snprintf(ph_s, sizeof(ph_s), "%llu", static_cast<unsigned long long>(ph.tick_idx));
    vn::log::info_s("videonode-source: shutting down",
                    {vn::key::real, real_s, vn::key::placeholder, ph_s});
    if (st.grpc_enabled) {
        st.grpc_svc.StopStreams();
        grpc_srv.Shutdown();
    }
    st.prod.stop();
    if (st.cap.active) {
        if (!st.cap.cap.stream_off()) {
            vn::log::error("videonode-source: STREAMOFF failed during shutdown (errno=%d)", errno);
        }
        teardown_session_(st.cap);
    }
    ph.destroy();
}

// Attempt to open capture at startup if device is configured.
void try_initial_capture_(CaptureSession& cap, source_probe::SourceProbe& probe, const Args& a,
                          nv12_buf::Allocator& allocator, auto& publish_active_format) {
    if (!a.device.empty()) {
        if (try_open_capture(cap, a, allocator) == CaptureOpenStatus::Ok) {
            probe.attach();
            publish_active_format(a);
        } else {
            vn::log::warn("videonode-source: capture not ready at startup");
        }
    }
}

// Main event loop: process frames, poll for events, handle state transitions.
void main_loop_(std::atomic<bool>& running, LoopState& st, nv12_buf::Allocator& allocator,
                auto& publish_active_format, auto& clear_active_format) {
    using clock = std::chrono::steady_clock;
    auto loop_start = clock::now();

    while (running.load()) {
        if (st.a.run_seconds > 0 &&
            clock::now() - loop_start > std::chrono::seconds(st.a.run_seconds))
            break;

        (void)st.prod.prune_dead_consumers();

        handle_format_change_(st, allocator, publish_active_format, clear_active_format);
        maybe_reinit_capture_(st, allocator, publish_active_format);
        poll_and_dispatch_(st);

        if (clock::now() >= st.next_power_poll) {
            st.probe.refresh_power_present();
            st.next_power_poll = clock::now() + std::chrono::seconds(1);
        }

        source_probe::Health h = st.probe.health();
        bool health_changed = (h != st.prev_health);
        if (health_changed) {
            vn::log::info_s("videonode-source: state change",
                            {vn::key::state, source_probe::status_text(h)});
            st.prev_health = h;
        }

        maybe_publish_status_(st, h, health_changed, st.ph.tick_idx);

        if (clock::now() < st.next_broadcast)
            continue;

        if (h == source_probe::Health::Live) {
            st.next_broadcast += st.broadcast_period;
            // Catch up if we've fallen behind (e.g. real frames stalled while
            // health still reads Live) so the deadline never sits in the past,
            // which would collapse the poll timeout to 0 and busy-spin.
            if (st.next_broadcast < clock::now())
                st.next_broadcast = clock::now() + st.broadcast_period;
            continue;
        }

        broadcast_tick_(st, h);
        st.next_broadcast += st.broadcast_period;
        if (st.next_broadcast < clock::now())
            st.next_broadcast = clock::now() + st.broadcast_period;
    }
}

} // namespace

int Run(const Args& a_in, std::atomic<bool>& running) {
    // Local mutable copy: set_format requests at runtime mutate in_format/
    // width/height/fps for the reinit-with-new-args path below.
    Args a = a_in;

    nv12_buf::Allocator allocator;
    if (!init_allocator_(allocator))
        return 1;

    PlaceholderRing ph;
    if (!ph.init(allocator, a.placeholder_w, a.placeholder_h, a.device)) {
        vn::log::fatal("videonode-source: failed to allocate placeholder ring");
        return 1;
    }
    scm_rights_producer::ScmRightsProducer prod;
    scm_rights_producer::InitParams pp;
    pp.socket_path = a.out_socket;
    pp.max_consumers = a.max_consumers;
    if (!prod.init(pp) || !prod.start())
        return 1;

    CaptureSession cap;
    source_probe::SourceProbe probe(cap.cap);

    bool need_reinit_for_format_change = false;
    std::optional<nativerpc::ActiveFormat> active_format;
    nativerpc::SourceContext gctx;
    populate_gctx_(gctx, running, a, need_reinit_for_format_change, probe, active_format);

    auto publish_active_format = [&]([[maybe_unused]] const Args& used) {
        std::lock_guard<std::mutex> lock(gctx.set_format_mu);
        // fps comes from the actual negotiated rate (VIDIOC_G_PARM via
        // get_format), not the requested arg — so the SetFormat no-op
        // compares against reality and pins a rate the device wasn't using.
        active_format = nativerpc::ActiveFormat{.fourcc = cap.src_fmt_name,
                                                .w = static_cast<uint32_t>(cap.width),
                                                .h = static_cast<uint32_t>(cap.height),
                                                .fps = cap.fps};
    };
    auto clear_active_format = [&]() {
        std::lock_guard<std::mutex> lock(gctx.set_format_mu);
        active_format.reset();
    };

    try_initial_capture_(cap, probe, a, allocator, publish_active_format);

    nativerpc::SourceService grpc_svc(&gctx);
    nativerpc::GrpcServer grpc_srv;
    bool grpc_enabled = false;
    start_grpc_(a, grpc_svc, grpc_srv, grpc_enabled);

    using clock = std::chrono::steady_clock;
    const auto broadcast_period =
        std::chrono::nanoseconds(1'000'000'000LL / std::max(1, a.placeholder_broadcast_fps));

    LoopState st{.ph = ph,
                 .prod = prod,
                 .cap = cap,
                 .probe = probe,
                 .grpc_svc = grpc_svc,
                 .gctx = gctx,
                 .a = a,
                 .grpc_enabled = grpc_enabled,
                 .need_reinit = !cap.active,
                 .next_broadcast = clock::now(),
                 .next_power_poll = clock::now(),
                 .next_status_heartbeat = clock::now(),
                 .next_reinit_attempt = clock::now(),
                 .broadcast_period = broadcast_period};

    main_loop_(running, st, allocator, publish_active_format, clear_active_format);

    shutdown_(st, grpc_srv, ph);
    return 0;
}

} // namespace source
