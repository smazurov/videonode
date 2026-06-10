#include "src/source/orchestrator.hpp"

#include "control/common.pb.h"
#include "src/capture/source_probe.hpp"
#include "src/common/log_keys.hpp"
#include "src/common/log_levels.hpp"
#include "src/ipc/scm_rights_producer.hpp"
#include "src/render/nv12_buf.hpp"
#include "src/rpc/grpc_server.hpp"
#include "src/source/broadcast.hpp"
#include "src/source/capture_session.hpp"
#include "src/source/pipe_session.hpp"
#include "src/source/placeholder_ring.hpp"
#include "src/source/source_runtime.hpp"
#include "src/source/source_service.hpp"

#include <algorithm>
#include <atomic>
#include <chrono>
#include <cstdio>
#include <mutex>
#include <optional>
#include <poll.h>
#include <thread>

namespace source {

namespace {

using pipe_clock = std::chrono::steady_clock;

struct PipeLoopState {
    PlaceholderRing& ph;
    scm_rights_producer::ScmRightsProducer& prod;
    PipeSession& session;
    CaptureSession& cap;
    source_probe::SourceProbe& probe;
    nativerpc::SourceService& grpc_svc;
    nativerpc::SourceContext& gctx;
    Args& a;
    bool grpc_enabled;

    uint64_t real_frame_idx = 0;
    source_probe::Health prev_health = source_probe::Health::Probing;
    int prev_consumer_count = -1;
    pipe_clock::time_point next_broadcast;
    pipe_clock::time_point next_status_heartbeat;
    std::chrono::nanoseconds broadcast_period{};
    uint32_t pipe_w = 0;
    uint32_t pipe_h = 0;
    uint32_t pipe_fps = 0;
};

dmabuf_header::ColorMatrix pipe_matrix(uint32_t height) {
    return height >= 720 ? dmabuf_header::ColorMatrix::Bt709 : dmabuf_header::ColorMatrix::Bt601;
}

void publish_detected_format_(PipeLoopState& st) {
    const Y4mFormat& f = st.session.format();
    st.pipe_w = static_cast<uint32_t>(f.width);
    st.pipe_h = static_cast<uint32_t>(f.height);
    st.pipe_fps = f.fps();
    {
        std::lock_guard<std::mutex> lock(st.gctx.set_format_mu);
        *st.gctx.active_format = nativerpc::ActiveFormat{
            .fourcc = "NV12", .w = st.pipe_w, .h = st.pipe_h, .fps = st.pipe_fps};
    }
    // 1 s floor: ffmpeg's -re stream_loop boundary pauses ~0.6 s; dead children are caught by EOF.
    st.probe.set_stall_deadline(std::max<pipe_clock::duration>(
        source_probe::SourceProbe::stall_deadline_for_fps(st.pipe_fps), std::chrono::seconds(1)));
}

void clear_detected_format_(PipeLoopState& st) {
    std::lock_guard<std::mutex> lock(st.gctx.set_format_mu);
    st.gctx.active_format->reset();
}

void handle_event_(PipeLoopState& st, PipeSession::Event ev) {
    switch (ev) {
    case PipeSession::Event::None:
        return;
    case PipeSession::Event::ChildStarted:
        st.probe.note_device_acquiring();
        return;
    case PipeSession::Event::FormatDetected:
        publish_detected_format_(st);
        return;
    case PipeSession::Event::Frame:
        st.probe.note_dqbuf_success(pipe_clock::now());
        ++st.real_frame_idx;
        broadcast_buffer(st.prod, st.session.last_frame(), st.real_frame_idx,
                         pipe_matrix(st.pipe_h));
        if (st.grpc_enabled)
            st.grpc_svc.UpdateLastFrame(make_frame_ref(st.session.last_frame(), st.real_frame_idx));
        st.next_broadcast = pipe_clock::now() + st.broadcast_period;
        return;
    case PipeSession::Event::ChildDown:
        st.probe.note_device_absent();
        clear_detected_format_(st);
        return;
    }
}

void poll_pipe_(PipeLoopState& st) {
    if (st.session.fd() < 0) {
        std::this_thread::sleep_until(st.next_broadcast);
        return;
    }
    auto until_next = st.next_broadcast - pipe_clock::now();
    int timeout_ms = int(std::chrono::duration_cast<std::chrono::milliseconds>(until_next).count());
    timeout_ms = std::clamp(timeout_ms, 0, 100);

    pollfd pfd{};
    pfd.fd = st.session.fd();
    pfd.events = POLLIN;
    if (::poll(&pfd, 1, timeout_ms) <= 0)
        return;
    handle_event_(st, st.session.consume());
}

void publish_status_(PipeLoopState& st, source_probe::Health h, bool health_changed) {
    if (!st.grpc_enabled)
        return;
    const int cur_consumers = st.prod.consumer_count();
    const bool consumers_changed = (cur_consumers != st.prev_consumer_count);
    const bool heartbeat_due = pipe_clock::now() >= st.next_status_heartbeat;
    if (!health_changed && !consumers_changed && !heartbeat_due)
        return;
    videonode::control::Status sp;
    StatusContext ctx{.device_id = st.a.device_id,
                      .probe = st.probe,
                      .health = h,
                      .cap = st.cap,
                      .args = st.a,
                      .real_frame_idx = st.real_frame_idx,
                      .placeholder_frames = st.ph.tick_idx,
                      .last_seq = 0,
                      .prod = st.prod,
                      .pipe_w = st.pipe_w,
                      .pipe_h = st.pipe_h,
                      .pipe_fps = st.pipe_fps};
    build_status_proto(sp, ctx);
    st.grpc_svc.PublishStatus(sp);
    st.prev_consumer_count = cur_consumers;
    st.next_status_heartbeat = pipe_clock::now() + std::chrono::seconds(1);
}

void placeholder_tick_(PipeLoopState& st, source_probe::Health h) {
    nv12_buf::Buffer& ph_buf = st.ph.paint_and_pick(now_ms(), source_probe::status_text(h));
    nv12_buf::stage_for_read(ph_buf);
    broadcast_buffer(st.prod, ph_buf, st.ph.tick_idx);
    if (st.grpc_enabled)
        st.grpc_svc.UpdateLastFrame(make_frame_ref(ph_buf, st.ph.tick_idx));
}

void pipe_main_loop_(std::atomic<bool>& running, PipeLoopState& st) {
    auto loop_start = pipe_clock::now();
    while (running.load()) {
        if (st.a.run_seconds > 0 &&
            pipe_clock::now() - loop_start > std::chrono::seconds(st.a.run_seconds))
            break;

        (void)st.prod.prune_dead_consumers();

        handle_event_(st, st.session.tick(pipe_clock::now()));
        poll_pipe_(st);

        st.probe.note_tick(pipe_clock::now());
        source_probe::Health h = st.probe.health();
        bool health_changed = (h != st.prev_health);
        if (health_changed) {
            vn::log::info_s("videonode-source: state change",
                            {vn::key::state, source_probe::health_token(h)});
            st.prev_health = h;
        }
        publish_status_(st, h, health_changed);

        if (pipe_clock::now() < st.next_broadcast)
            continue;
        if (h == source_probe::Health::Live) {
            st.next_broadcast += st.broadcast_period;
            if (st.next_broadcast < pipe_clock::now())
                st.next_broadcast = pipe_clock::now() + st.broadcast_period;
            continue;
        }
        placeholder_tick_(st, h);
        st.next_broadcast += st.broadcast_period;
        if (st.next_broadcast < pipe_clock::now())
            st.next_broadcast = pipe_clock::now() + st.broadcast_period;
    }
}

} // namespace

int RunPipe(const Args& a_in, std::atomic<bool>& running) {
    Args a = a_in;
    if (!a.device.empty()) {
        vn::log::fatal("videonode-source: --pipe-cmd and --device are mutually exclusive");
        return 1;
    }

    nv12_buf::Allocator allocator;
    if (!init_allocator(allocator))
        return 1;

    PlaceholderRing ph;
    if (!ph.init(allocator, a.placeholder_w, a.placeholder_h, "pipe")) {
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
    populate_gctx(gctx, running, a, need_reinit_for_format_change, probe, active_format);

    nativerpc::SourceService grpc_svc(&gctx);
    nativerpc::GrpcServer grpc_srv;
    bool grpc_enabled = false;
    (void)start_grpc(a, grpc_svc, grpc_srv, grpc_enabled);

    PipeSession session;
    session.init(allocator, a.pipe_cmd);

    const auto broadcast_period =
        std::chrono::nanoseconds(1'000'000'000LL / std::max(1, a.placeholder_broadcast_fps));

    PipeLoopState st{.ph = ph,
                     .prod = prod,
                     .session = session,
                     .cap = cap,
                     .probe = probe,
                     .grpc_svc = grpc_svc,
                     .gctx = gctx,
                     .a = a,
                     .grpc_enabled = grpc_enabled,
                     .next_broadcast = pipe_clock::now(),
                     .next_status_heartbeat = pipe_clock::now(),
                     .broadcast_period = broadcast_period};

    pipe_main_loop_(running, st);

    char real_s[24], ph_s[24];
    std::snprintf(real_s, sizeof(real_s), "%llu",
                  static_cast<unsigned long long>(st.real_frame_idx));
    std::snprintf(ph_s, sizeof(ph_s), "%llu", static_cast<unsigned long long>(ph.tick_idx));
    vn::log::info_s("videonode-source: shutting down",
                    {vn::key::real, real_s, vn::key::placeholder, ph_s});
    if (grpc_enabled) {
        grpc_svc.StopStreams();
        grpc_srv.Shutdown();
    }
    prod.stop();
    session.stop();
    ph.destroy();
    return 0;
}

} // namespace source
