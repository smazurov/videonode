// broadcast implementation. See broadcast.hpp.

#include "src/source/broadcast.hpp"

#include "src/rpc/dmabuf_msg.hpp"

#include <chrono>
#include <cstdio>
#include <sstream>

namespace source {

namespace {

// json_quote escapes a string for safe injection into a JSON object as
// a literal value. Mirrors control_channel.cpp's helper but kept local
// to avoid leaking that one outside the channel.
std::string json_quote(const std::string& s) {
    std::string o;
    o.reserve(s.size() + 2);
    o += '"';
    for (char c : s) {
        switch (c) {
        case '"':
            o += "\\\"";
            break;
        case '\\':
            o += "\\\\";
            break;
        case '\n':
            o += "\\n";
            break;
        case '\t':
            o += "\\t";
            break;
        case '\r':
            o += "\\r";
            break;
        default:
            if (static_cast<unsigned char>(c) < 0x20) {
                char buf[8];
                std::snprintf(buf, sizeof(buf), "\\u%04x", static_cast<unsigned>(c));
                o += buf;
            } else {
                o += c;
            }
            break;
        }
    }
    o += '"';
    return o;
}

} // namespace

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
    // Color contract — see dmabuf_msg.hpp. RGA's IM_COLOR_SPACE_DEFAULT
    // and csc_gles's BT.601 shader both emit BT.601 limited / MPEG-2.
    h_.color_matrix = dmabuf_msg::ColorMatrix::Bt601;
    h_.color_range = dmabuf_msg::ColorRange::Limited;
    h_.chroma_siting = dmabuf_msg::ChromaSiting::Mpeg2;
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

std::string build_status_params(const std::string& device_id, source_probe::SourceProbe& probe,
                                source_probe::Health h, const CaptureSession& cap, const Args& a,
                                uint64_t real_frame_idx, uint64_t placeholder_frames,
                                uint32_t last_seq, scm_rights_producer::ScmRightsProducer& prod) {
    std::ostringstream o;
    o << "{";
    o << "\"device_id\":" << json_quote(device_id);
    o << ",\"ts_ms\":" << now_ms();
    o << ",\"health\":" << json_quote(source_probe::status_text(h));

    o << ",\"device\":{";
    o << "\"path\":" << json_quote(a.device);
    o << ",\"multiplanar\":" << (cap.active && cap.cap.multiplanar() ? "true" : "false");
    o << "}";

    o << ",\"signal\":{";
    o << "\"has_dv_timings\":" << (probe.has_dv_timings() ? "true" : "false");
    o << ",\"cable_present\":" << (probe.cable_present() ? "true" : "false");
    o << ",\"signal_locked\":" << (probe.signal_locked() ? "true" : "false");
    o << ",\"dv_timings\":"
      << json_quote(source_probe::SourceProbe::dv_timings_label_public(probe.dv_timings_state()));
    o << "}";

    o << ",\"format\":{";
    if (cap.active) {
        o << "\"fourcc\":" << json_quote(cap.src_fmt_name);
        o << ",\"w\":" << cap.width;
        o << ",\"h\":" << cap.height;
        o << ",\"fps\":" << a.in_fps;
        o << ",\"buffers\":" << cap.cap.buffers().size();
        const char* mode_name = (cap.mode == DecodeMode::Mjpeg)
                                    ? (cap.using_mpp ? "mjpeg-mpp" : "mjpeg-turbojpeg")
                                    : "rga";
        o << ",\"mode\":" << json_quote(mode_name);
    } else {
        o << "\"fourcc\":\"\",\"w\":0,\"h\":0,\"fps\":0,\"buffers\":0,\"mode\":\"\"";
    }
    o << "}";

    o << ",\"broadcast\":{";
    o << "\"target_fps\":" << a.broadcast_fps;
    o << ",\"real_frames\":" << real_frame_idx;
    o << ",\"placeholder_frames\":" << placeholder_frames;
    o << ",\"last_seq\":" << last_seq;
    o << "}";

    auto stats = prod.stats();
    o << ",\"consumers\":{";
    o << "\"count\":" << prod.consumer_count();
    o << ",\"live\":[";
    bool first = true;
    for (const auto& cs : stats) {
        if (cs.evicted_at_frame != 0)
            continue;
        if (!first)
            o << ",";
        first = false;
        o << "{\"fd\":" << cs.fd << ",\"frames_sent\":" << cs.frames_sent
          << ",\"frames_dropped\":" << cs.frames_dropped << "}";
    }
    o << "],\"evicted\":[";
    first = true;
    for (const auto& cs : stats) {
        if (cs.evicted_at_frame == 0)
            continue;
        if (!first)
            o << ",";
        first = false;
        o << "{\"fd\":" << cs.fd << ",\"frames_sent\":" << cs.frames_sent
          << ",\"frames_dropped\":" << cs.frames_dropped
          << ",\"evicted_at_frame\":" << cs.evicted_at_frame << "}";
    }
    o << "]}";

    o << "}";
    return o.str();
}

} // namespace source
