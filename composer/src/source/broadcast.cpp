#include "src/source/broadcast.hpp"

#include "control/common.pb.h"
#include "src/ipc/dmabuf_header.hpp"

#include <chrono>

namespace source {

uint64_t now_ms() {
    using namespace std::chrono;
    return duration_cast<milliseconds>(steady_clock::now().time_since_epoch()).count();
}

vn::snapshot::FrameRef make_frame_ref(const jpeg_dec::DecodedNv12& d, uint64_t frame_idx) {
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
    using namespace std::chrono;
    r.captured_at_ns = static_cast<uint64_t>(
        duration_cast<nanoseconds>(steady_clock::now().time_since_epoch()).count());
    r.slot_index = d.slot_index;
    r.generation = d.generation;
    return r;
}

vn::snapshot::FrameRef make_frame_ref(const nv12_buf::Buffer& b, uint64_t frame_idx) {
    jpeg_dec::DecodedNv12 d;
    d.fd = b.y_fd;
    d.plane1_fd = b.uv_fd;
    d.width = b.width;
    d.height = b.height;
    d.y_pitch = b.y_pitch;
    d.uv_pitch = b.uv_pitch;
    d.y_offset = b.y_offset;
    d.uv_offset = b.uv_offset;
    return make_frame_ref(d, frame_idx);
}

int64_t wall_ms() {
    using namespace std::chrono;
    return duration_cast<milliseconds>(system_clock::now().time_since_epoch()).count();
}

int broadcast_nv12(scm_rights_producer::ScmRightsProducer& prod, const jpeg_dec::DecodedNv12& d,
                   uint64_t frame_idx, dmabuf_header::ColorMatrix matrix) {
    dmabuf_header::Header h_;
    h_.slot_index = d.slot_index;
    h_.generation = d.generation;
    h_.width = uint32_t(d.width);
    h_.height = uint32_t(d.height);
    h_.format = "NV12";
    h_.plane_pitches = {d.y_pitch, d.uv_pitch};
    h_.plane_offsets = {d.y_offset, d.uv_offset};
    // Limited range / MPEG-2 siting are fixed; the matrix is the detected
    // capture colorimetry (samples pass through CSC matrix-preserved).
    h_.color_matrix = matrix;
    h_.color_range = dmabuf_header::ColorRange::Limited;
    h_.chroma_siting = dmabuf_header::ChromaSiting::Mpeg2;
    h_.frame_idx = frame_idx;
    int uv_fd = d.plane1_fd >= 0 ? d.plane1_fd : d.fd;
    return prod.broadcast(h_, {d.fd, uv_fd});
}

void broadcast_buffer(scm_rights_producer::ScmRightsProducer& prod, const nv12_buf::Buffer& b,
                      uint64_t frame_idx, dmabuf_header::ColorMatrix matrix) {
    jpeg_dec::DecodedNv12 d;
    d.fd = (b.staged_y_fd >= 0) ? b.staged_y_fd : b.y_fd;
    d.plane1_fd = (b.staged_uv_fd >= 0) ? b.staged_uv_fd : b.uv_fd;
    d.width = b.width;
    d.height = b.height;
    d.y_pitch = b.y_pitch;
    d.uv_pitch = b.uv_pitch;
    d.y_offset = (b.staged_y_fd >= 0) ? 0 : b.y_offset;
    d.uv_offset = (b.staged_uv_fd >= 0) ? 0 : b.uv_offset;
    broadcast_nv12(prod, d, frame_idx, matrix);
}

void build_status_proto(::videonode::control::Status& out, const StatusContext& ctx) {
    out.Clear();
    out.set_device_id(ctx.device_id);
    out.set_ts_ms(wall_ms());
    // A device-less test_mode source has no V4L2 frames to lock onto, so the
    // probe sits at Probing; report it as live since the test producer is up.
    // Pipe sources DO have real frame flow, so they report true probe health.
    const bool test_mode = ctx.args.device.empty() && ctx.args.pipe_cmd.empty();
    out.set_health(test_mode ? "live" : source_probe::health_token(ctx.health));

    auto* dev = out.mutable_device();
    dev->set_path(ctx.args.device);
    dev->set_multiplanar(ctx.cap.active && ctx.cap.cap.multiplanar());

    auto* sig = out.mutable_signal();
    sig->set_has_dv_timings(ctx.probe.has_dv_timings());
    sig->set_cable_present(ctx.probe.cable_present());
    sig->set_signal_locked(ctx.probe.signal_locked());
    sig->set_dv_timings(
        source_probe::SourceProbe::dv_timings_label_public(ctx.probe.dv_timings_state()));

    auto* fmt = out.mutable_format();
    if (ctx.cap.active) {
        fmt->set_fourcc(ctx.cap.src_fmt_name);
        fmt->set_w(static_cast<uint32_t>(ctx.cap.width));
        fmt->set_h(static_cast<uint32_t>(ctx.cap.height));
        fmt->set_fps(ctx.cap.fps); // actual negotiated rate, not the requested arg
        fmt->set_buffers(static_cast<uint32_t>(ctx.cap.cap.buffers().size()));
        const char* mode_name = (ctx.cap.mode == DecodeMode::Mjpeg)
                                    ? (ctx.cap.using_mpp ? "mjpeg-mpp" : "mjpeg-turbojpeg")
                                    : "rga";
        fmt->set_mode(mode_name);
        fmt->set_color_matrix(ctx.cap.color_matrix == v4l2::ColorMatrix::Bt709 ? "bt709" : "bt601");
    } else if (!ctx.args.pipe_cmd.empty() && ctx.pipe_w > 0) {
        fmt->set_fourcc("NV12");
        fmt->set_w(ctx.pipe_w);
        fmt->set_h(ctx.pipe_h);
        fmt->set_fps(ctx.pipe_fps);
        fmt->set_buffers(3);
        fmt->set_mode("pipe");
        // Matches the broadcast header's height-based matrix pick.
        fmt->set_color_matrix(ctx.pipe_h >= 720 ? "bt709" : "bt601");
    } else {
        // V4L2 not negotiated (test_mode sources, or capture still
        // initialising). Broadcasts are NV12 placeholder frames at
        // placeholder_w × placeholder_h; report those dims so consumers
        // (UI AR/crop, etc.) can size against the real frame stream
        // instead of zeros.
        fmt->set_fourcc("NV12");
        fmt->set_w(static_cast<uint32_t>(ctx.args.placeholder_w));
        fmt->set_h(static_cast<uint32_t>(ctx.args.placeholder_h));
        fmt->set_fps(static_cast<uint32_t>(ctx.args.placeholder_broadcast_fps));
        fmt->set_mode("placeholder");
        // Leave color_matrix unset: no real signal to detect yet, so the
        // daemon falls back to its own resolution heuristic instead of being
        // pinned to a guess that may mislabel a 1080p source.
    }

    auto* bc = out.mutable_broadcast();
    bc->set_target_fps(static_cast<uint32_t>(ctx.args.placeholder_broadcast_fps));
    bc->set_real_frames(ctx.real_frame_idx);
    bc->set_placeholder_frames(ctx.placeholder_frames);
    bc->set_last_seq(ctx.last_seq);

    auto* cons = out.mutable_consumers();
    auto stats = ctx.prod.stats();
    cons->set_count(ctx.prod.consumer_count());
    for (const auto& cs : stats) {
        if (cs.evicted_at_frame != 0) {
            auto* row = cons->add_evicted();
            row->set_fd(cs.fd);
            row->set_frames_sent(cs.frames_sent);
            row->set_frames_dropped(cs.frames_dropped);
            row->set_evicted_at_frame(cs.evicted_at_frame);
        } else {
            auto* row = cons->add_live();
            row->set_fd(cs.fd);
            row->set_frames_sent(cs.frames_sent);
            row->set_frames_dropped(cs.frames_dropped);
        }
    }
}

} // namespace source
