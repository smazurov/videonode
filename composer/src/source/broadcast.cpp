// broadcast implementation. See broadcast.hpp.

#include "src/source/broadcast.hpp"

#include "control/common.pb.h"
#include "src/ipc/dmabuf_header.hpp"

#include <chrono>

namespace source {

uint64_t now_ms() {
    using namespace std::chrono;
    return duration_cast<milliseconds>(steady_clock::now().time_since_epoch()).count();
}

void broadcast_nv12(scm_rights_producer::ScmRightsProducer& prod, const jpeg_dec::DecodedNv12& d,
                    uint64_t frame_idx) {
    dmabuf_header::Header h_;
    h_.slot_index = 0;
    h_.width = uint32_t(d.width);
    h_.height = uint32_t(d.height);
    h_.format = "NV12";
    h_.plane_pitches = {d.y_pitch, d.uv_pitch};
    h_.plane_offsets = {d.y_offset, d.uv_offset};
    // Color contract — see ipc/dmabuf_header.hpp. RGA's
    // IM_COLOR_SPACE_DEFAULT and csc_gles's BT.601 shader both emit
    // BT.601 limited / MPEG-2.
    h_.color_matrix = dmabuf_header::ColorMatrix::Bt601;
    h_.color_range = dmabuf_header::ColorRange::Limited;
    h_.chroma_siting = dmabuf_header::ChromaSiting::Mpeg2;
    h_.frame_idx = frame_idx;
    int uv_fd = d.plane1_fd >= 0 ? d.plane1_fd : d.fd;
    prod.broadcast(h_, {d.fd, uv_fd});
}

void broadcast_buffer(scm_rights_producer::ScmRightsProducer& prod, const nv12_buf::Buffer& b,
                      uint64_t frame_idx) {
    jpeg_dec::DecodedNv12 d;
    d.fd = (b.staged_y_fd >= 0) ? b.staged_y_fd : b.y_fd;
    d.plane1_fd = (b.staged_uv_fd >= 0) ? b.staged_uv_fd : b.uv_fd;
    d.width = b.width;
    d.height = b.height;
    d.y_pitch = b.y_pitch;
    d.uv_pitch = b.uv_pitch;
    d.y_offset = (b.staged_y_fd >= 0) ? 0 : b.y_offset;
    d.uv_offset = (b.staged_uv_fd >= 0) ? 0 : b.uv_offset;
    broadcast_nv12(prod, d, frame_idx);
}

void build_status_proto(::videonode::control::Status& out, const std::string& device_id,
                        source_probe::SourceProbe& probe, source_probe::Health h,
                        const CaptureSession& cap, const Args& a, uint64_t real_frame_idx,
                        uint64_t placeholder_frames, uint32_t last_seq,
                        scm_rights_producer::ScmRightsProducer& prod) {
    out.Clear();
    out.set_device_id(device_id);
    out.set_ts_ms(static_cast<int64_t>(now_ms()));
    out.set_health(source_probe::status_text(h));

    auto* dev = out.mutable_device();
    dev->set_path(a.device);
    dev->set_multiplanar(cap.active && cap.cap.multiplanar());

    auto* sig = out.mutable_signal();
    sig->set_has_dv_timings(probe.has_dv_timings());
    sig->set_cable_present(probe.cable_present());
    sig->set_signal_locked(probe.signal_locked());
    sig->set_dv_timings(
        source_probe::SourceProbe::dv_timings_label_public(probe.dv_timings_state()));

    auto* fmt = out.mutable_format();
    if (cap.active) {
        fmt->set_fourcc(cap.src_fmt_name);
        fmt->set_w(static_cast<uint32_t>(cap.width));
        fmt->set_h(static_cast<uint32_t>(cap.height));
        fmt->set_fps(static_cast<uint32_t>(a.in_fps));
        fmt->set_buffers(static_cast<uint32_t>(cap.cap.buffers().size()));
        const char* mode_name = (cap.mode == DecodeMode::Mjpeg)
                                    ? (cap.using_mpp ? "mjpeg-mpp" : "mjpeg-turbojpeg")
                                    : "rga";
        fmt->set_mode(mode_name);
    }

    auto* bc = out.mutable_broadcast();
    bc->set_target_fps(static_cast<uint32_t>(a.placeholder_broadcast_fps));
    bc->set_real_frames(real_frame_idx);
    bc->set_placeholder_frames(placeholder_frames);
    bc->set_last_seq(last_seq);

    auto* cons = out.mutable_consumers();
    auto stats = prod.stats();
    cons->set_count(prod.consumer_count());
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
