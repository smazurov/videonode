// broadcast implementation. See broadcast.hpp.

#include "src/source/broadcast.hpp"

#include "control/common.pb.h"
#include "src/ipc/dmabuf_header.hpp"
#include "src/source/source_service.hpp" // nativerpc::LatestFrame

#include <chrono>
#include <cstring>
#include <sys/mman.h>
#include <unistd.h>

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
    d.fd = b.y_fd;
    d.plane1_fd = b.uv_fd;
    d.width = b.width;
    d.height = b.height;
    d.y_pitch = b.y_pitch;
    d.uv_pitch = b.uv_pitch;
    d.y_offset = b.y_offset;
    d.uv_offset = b.uv_offset;
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

namespace {

uint64_t now_ns_monotonic() {
    using namespace std::chrono;
    return static_cast<uint64_t>(
        duration_cast<nanoseconds>(steady_clock::now().time_since_epoch()).count());
}

// Copy `count` bytes out of a dma-buf mapped at `base + offset` into `dst`
// at `dst_offset`. Returns false if the dma-buf can't be mmap'd or is
// shorter than offset+count.
// copy_packed copies `rows` rows of `width` bytes each from a
// pitch-strided source plane into a tightly-packed destination. The
// snapshot wire format (`SnapshotResponse.nv12`) is width-packed so
// the daemon's ffmpeg subprocess at `-s WxH` interprets each row
// correctly regardless of dma-buf allocator padding. Drops the trailing
// (pitch - width) bytes of each row.
bool copy_packed(int fd, size_t offset, size_t pitch, size_t width, size_t rows,
                 std::vector<uint8_t>& dst, size_t dst_offset) {
    if (fd < 0 || pitch < width) {
        return false;
    }
    const size_t map_size = offset + pitch * rows;
    void* mapped = ::mmap(nullptr, map_size, PROT_READ, MAP_SHARED, fd, 0);
    if (mapped == MAP_FAILED) {
        return false;
    }
    const auto* src = static_cast<const uint8_t*>(mapped) + offset;
    uint8_t* d = dst.data() + dst_offset;
    if (pitch == width) {
        // Pitch matches width — single bulk memcpy is fine.
        std::memcpy(d, src, pitch * rows);
    } else {
        // Strided source; pack into the destination row by row.
        for (size_t r = 0; r < rows; ++r) {
            std::memcpy(d + r * width, src + r * pitch, width);
        }
    }
    ::munmap(mapped, map_size);
    return true;
}

} // namespace

bool snapshot_nv12_from_decoded(const jpeg_dec::DecodedNv12& d, uint64_t frame_idx,
                                ::nativerpc::LatestFrame& out) {
    if (d.width <= 0 || d.height <= 0 || d.y_pitch == 0 || d.uv_pitch == 0) {
        return false;
    }
    const size_t width = static_cast<size_t>(d.width);
    const size_t height = static_cast<size_t>(d.height);
    if (d.y_pitch < width || d.uv_pitch < width) {
        return false;
    }
    // Width-packed NV12: Y plane is width*height bytes, UV plane is
    // width*(height/2) bytes (interleaved Cb/Cr at half-res).
    const size_t y_bytes = width * height;
    const size_t uv_bytes = width * (height / 2);
    out.nv12.resize(y_bytes + uv_bytes);
    if (!copy_packed(d.fd, d.y_offset, d.y_pitch, width, height, out.nv12, 0)) {
        return false;
    }
    const int uv_fd = d.plane1_fd >= 0 ? d.plane1_fd : d.fd;
    if (!copy_packed(uv_fd, d.uv_offset, d.uv_pitch, width, height / 2, out.nv12, y_bytes)) {
        return false;
    }
    out.width = static_cast<uint32_t>(d.width);
    out.height = static_cast<uint32_t>(d.height);
    // Report the source pitches in case a future consumer wants them;
    // the bytes themselves are width-packed.
    out.pitch_y = d.y_pitch;
    out.pitch_uv = d.uv_pitch;
    out.frame_idx = frame_idx;
    out.captured_at_ns = now_ns_monotonic();
    return true;
}

bool snapshot_nv12_from_buffer(const nv12_buf::Buffer& b, uint64_t frame_idx,
                               ::nativerpc::LatestFrame& out) {
    jpeg_dec::DecodedNv12 d;
    d.fd = b.y_fd;
    d.plane1_fd = b.uv_fd;
    d.width = b.width;
    d.height = b.height;
    d.y_pitch = b.y_pitch;
    d.uv_pitch = b.uv_pitch;
    d.y_offset = b.y_offset;
    d.uv_offset = b.uv_offset;
    return snapshot_nv12_from_decoded(d, frame_idx, out);
}

} // namespace source
