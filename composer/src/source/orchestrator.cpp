#include "src/source/orchestrator.hpp"

#include "control/common.pb.h"
#include "src/capture/signal_activity.hpp"
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
    std::vector<uint8_t> stage_;
    int next = 0;
    uint64_t tick_idx = 0;

    bool init(nv12_buf::Allocator& alloc, int w, int h, const std::string& device_path) {
        width = w;
        height = h;
        const size_t tight = size_t(w) * h * 3 / 2;
        stage_.assign(tight, 0);
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
    nv12_buf::Buffer& paint_and_pick(uint64_t wallclock_ms, const char* status) {
        ++tick_idx;
        int idx = next;
        next = (next + 1) % int(bufs.size());
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

struct LoopState {
    using clock = std::chrono::steady_clock;

    PlaceholderRing& ph;
    scm_rights_producer::ScmRightsProducer& prod;
    CaptureSession& cap;
    source_probe::SourceProbe& probe;
    nativerpc::SourceService& grpc_svc;
    nativerpc::SourceContext& gctx;
    Args& a;
    nv12_buf::Allocator& allocator; // for lazy MPP-CSC ring allocation (NV16/NV24 sources)
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
    bool prev_content_dead = false;
    std::chrono::nanoseconds broadcast_period;
    signal_activity::Detector signal_detector;
};

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

std::optional<csc::PixelFormat> to_csc(jpeg_dec::PixelFormat f) {
    switch (f) {
    case jpeg_dec::PixelFormat::Nv12:
        return csc::PixelFormat::Nv12;
    case jpeg_dec::PixelFormat::Nv16:
        return csc::PixelFormat::Nv16;
    case jpeg_dec::PixelFormat::Nv24:
        return csc::PixelFormat::Nv24;
    }
    return std::nullopt;
}

csc::ConvertParams nv12_dst_params(const nv12_buf::Buffer& dst, int w, int h) {
    csc::ConvertParams p;
    p.fd = dst.y_fd;
    p.uv_fd = (dst.uv_fd != dst.y_fd) ? dst.uv_fd : -1;
    p.uv_wstride = int(dst.uv_pitch);
    p.fmt = csc::PixelFormat::Nv12;
    p.width = w;
    p.height = h;
    p.wstride = int(dst.y_pitch);
    return p;
}

void commit_converted_slot(CaptureSession& cap, nv12_buf::Buffer& dst, jpeg_dec::DecodedNv12& out) {
    cap.out_ring_write = (cap.out_ring_write + 1) % static_cast<uint32_t>(cap.out_ring.size());
    nv12_buf::stage_for_read(dst);
    out.fd = (dst.staged_y_fd >= 0) ? dst.staged_y_fd : dst.y_fd;
    out.plane1_fd = (dst.staged_uv_fd >= 0) ? dst.staged_uv_fd : dst.uv_fd;
    out.y_pitch = dst.y_pitch;
    out.uv_pitch = dst.uv_pitch;
    out.y_offset = (dst.staged_y_fd >= 0) ? 0 : dst.y_offset;
    out.uv_offset = (dst.staged_uv_fd >= 0) ? 0 : dst.uv_offset;
    out.pixel_format = jpeg_dec::PixelFormat::Nv12;
}

bool convert_mpp_to_nv12_(LoopState& st, jpeg_dec::DecodedNv12& decoded) {
    std::optional<csc::PixelFormat> src_fmt = to_csc(decoded.pixel_format);
    if (!src_fmt) {
        static std::atomic<bool> warned{false};
        if (!warned.exchange(true))
            vn::log::error(
                "videonode-source: MJPEG CSC: unmappable decoded format, dropping frame");
        return false;
    }
    if (!ensure_mpp_output_ring(st.cap, st.a, st.allocator)) {
        static std::atomic<bool> warned{false};
        if (!warned.exchange(true))
            vn::log::error("videonode-source: MJPEG CSC output ring alloc failed; "
                           "non-NV12 source cannot be converted, dropping frames");
        return false;
    }
    nv12_buf::Buffer& dst_buf = st.cap.out_ring[st.cap.out_ring_write];

    csc::ConvertParams src_p;
    src_p.fd = decoded.fd;
    src_p.fmt = *src_fmt;
    src_p.width = decoded.width;
    src_p.height = decoded.height;
    // RGA locates UV via wstride*hstride, so hstride is the padded vertical stride.
    src_p.wstride = int(decoded.y_pitch);
    src_p.hstride = int(decoded.ver_stride);

    csc::ConvertParams dst_p = nv12_dst_params(dst_buf, decoded.width, decoded.height);
    if (!csc::convert(src_p, dst_p)) {
        static std::atomic<bool> warned{false};
        if (!warned.exchange(true))
            vn::log::error("videonode-source: %s -> NV12 CSC failed; backend may not support "
                           "this subsampling, source produces no frames",
                           decoded.pixel_format == jpeg_dec::PixelFormat::Nv24 ? "NV24" : "NV16");
        return false;
    }

    commit_converted_slot(st.cap, dst_buf, decoded);
    return true;
}

void refine_matrix_(LoopState& st, jpeg_dec::Colorimetry c) {
    v4l2::ColorMatrix m;
    if (c == jpeg_dec::Colorimetry::Bt709)
        m = v4l2::ColorMatrix::Bt709;
    else if (c == jpeg_dec::Colorimetry::Bt601)
        m = v4l2::ColorMatrix::Bt601;
    else
        return;
    if (m == st.cap.color_matrix)
        return;
    vn::log::info("videonode-source: colorimetry refined %s -> %s (decoder signal)",
                  st.cap.color_matrix == v4l2::ColorMatrix::Bt709 ? "bt709" : "bt601",
                  m == v4l2::ColorMatrix::Bt709 ? "bt709" : "bt601");
    st.cap.color_matrix = m;
}

// Locate CPU-readable luma for no-signal detection: packed YUYV/UYVY are read
// from the raw input mmap; TurboJPEG NV12 from its mapped output slot. Returns
// nullopt when luma isn't cheaply CPU-accessible (e.g. MPP pool, GPU-only).
std::optional<signal_activity::LumaView>
frame_luma_(const LoopState& st, const jpeg_dec::DecodedNv12& decoded, uint32_t df_index) {
    signal_activity::LumaView v;
    if (st.cap.mode == DecodeMode::Rga) {
        if (st.cap.src_fmt != csc::PixelFormat::Yuyv && st.cap.src_fmt != csc::PixelFormat::Uyvy)
            return std::nullopt;
        if (df_index >= st.cap.in_maps.size() || !st.cap.in_maps[df_index])
            return std::nullopt;
        v.data = std::span<const uint8_t>(static_cast<const uint8_t*>(st.cap.in_maps[df_index]),
                                          st.cap.in_map_sizes[df_index]);
        v.width = st.cap.width;
        v.height = st.cap.height;
        v.row_pitch = st.cap.width * 2;
        v.pixel_stride = 2;
        v.sample_offset = (st.cap.src_fmt == csc::PixelFormat::Uyvy) ? 1 : 0;
        return v;
    }
    if (st.cap.using_mpp)
        return std::nullopt;
    for (size_t i = 0; i < st.cap.out_ring.size() && i < st.cap.out_y.size(); ++i) {
        if (st.cap.out_ring[i].y_fd != decoded.fd || !st.cap.out_y[i])
            continue;
        v.data = std::span<const uint8_t>(static_cast<const uint8_t*>(st.cap.out_y[i]),
                                          static_cast<size_t>(decoded.y_pitch) * decoded.height);
        v.width = decoded.width;
        v.height = decoded.height;
        v.row_pitch = static_cast<int>(decoded.y_pitch);
        return v;
    }
    return std::nullopt;
}

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

    st.probe.note_dqbuf_success(LoopState::clock::now());
    st.last_dqbuf_seq = df.sequence;

    bool ok = false;
    jpeg_dec::DecodedNv12 decoded;
    if (st.cap.mode == DecodeMode::Rga) {
        nv12_buf::Buffer& dst_buf = st.cap.out_ring[st.cap.out_ring_write];
        csc::ConvertParams src_p;
        src_p.fd = st.cap.cap.buffers()[df.index].primary_dma_buf();
        src_p.fmt = st.cap.src_fmt;
        src_p.width = st.cap.width;
        src_p.height = st.cap.height;
        csc::ConvertParams dst_p = nv12_dst_params(dst_buf, st.cap.width, st.cap.height);
        if (csc::convert(src_p, dst_p)) {
            decoded.width = st.cap.width;
            decoded.height = st.cap.height;
            commit_converted_slot(st.cap, dst_buf, decoded);
            ok = true;
        }
    } else { // DecodeMode::Mjpeg
        if (df.index < st.cap.in_maps.size() && df.bytesused > 0) {
            const auto* jpeg = static_cast<const uint8_t*>(st.cap.in_maps[df.index]);
            if (st.cap.jpeg->decode(std::span<const uint8_t>(jpeg, df.bytesused), decoded)) {
                // NV12 broadcasts zero-copy from the MPP pool; NV16/NV24 (4:2:2 /
                // 4:4:4 sources, e.g. the MACROSILICON dongle) get CSC'd to NV12
                // first so the wire/sink/snapshot stay NV12-only.
                ok = decoded.pixel_format == jpeg_dec::PixelFormat::Nv12 ||
                     convert_mpp_to_nv12_(st, decoded);
            }
        }
    }

    if (ok) {
        refine_matrix_(st, decoded.colorimetry);
        // UVC dongles keep streaming a dead frame with no signal; detect it
        // from content and let the placeholder take over instead of forwarding
        // black. The HDMI path trusts DV-timings, so skip detection there.
        bool dead = false;
        if (!st.probe.has_dv_timings()) {
            if (auto lv = frame_luma_(st, decoded, df.index))
                dead = st.signal_detector.update(signal_activity::compute_luma_stats(*lv));
            if (dead != st.prev_content_dead) {
                vn::log::info(dead ? "videonode-source: frames flowing but black/frozen content "
                                     "detected; switching to no_signal"
                                   : "videonode-source: live content resumed; leaving no_signal");
                st.prev_content_dead = dead;
            }
            st.probe.note_content_dead(dead);
        }
        if (!dead) {
            ++st.real_frame_idx;
            broadcast_nv12(st.prod, decoded, st.real_frame_idx,
                           to_header_matrix(st.cap.color_matrix));
            if (st.grpc_enabled)
                st.grpc_svc.UpdateLastFrame(make_frame_ref(decoded, st.real_frame_idx));
            st.last_good_decoded = decoded;
            st.next_broadcast = LoopState::clock::now() + st.broadcast_period;
        }
    }

    if (!st.cap.cap.queue_buffer(df.index)) {
        vn::log::warn("videonode-source: QBUF failed (idx=%u errno=%d); "
                      "kernel ring depth reduced, continuing",
                      df.index, errno);
    }

    return ok;
}

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

        st.probe.note_tick(clock::now());
        source_probe::Health h = st.probe.health();
        bool health_changed = (h != st.prev_health);
        if (health_changed) {
            vn::log::info_s("videonode-source: state change",
                            {vn::key::state, source_probe::health_token(h)});
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
        probe.set_stall_deadline(source_probe::SourceProbe::stall_deadline_for_fps(cap.fps));
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
                 .allocator = allocator,
                 .grpc_enabled = grpc_enabled,
                 .need_reinit = !cap.active,
                 .next_broadcast = clock::now(),
                 .next_power_poll = clock::now(),
                 .next_status_heartbeat = clock::now(),
                 .next_reinit_attempt = clock::now(),
                 .broadcast_period = broadcast_period,
                 .signal_detector = {}};

    main_loop_(running, st, allocator, publish_active_format, clear_active_format);

    shutdown_(st, grpc_srv, ph);
    return 0;
}

} // namespace source
