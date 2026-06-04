#pragma once

#include "src/capture/jpeg_dec.hpp"
#include "src/capture/source_probe.hpp"
#include "src/ipc/scm_rights_producer.hpp"
#include "src/render/nv12_buf.hpp"
#include "src/snapshot/snapshot.hpp"
#include "src/source/args.hpp"
#include "src/source/capture_session.hpp"

#include <cstdint>
#include <string>

namespace source {

vn::snapshot::FrameRef make_frame_ref(const jpeg_dec::DecodedNv12& d, uint64_t frame_idx);
vn::snapshot::FrameRef make_frame_ref(const nv12_buf::Buffer& b, uint64_t frame_idx);

uint64_t now_ms();

// Not clock-jump-immune; use now_ms() for interval math.
int64_t wall_ms();

inline dmabuf_header::ColorMatrix to_header_matrix(v4l2::ColorMatrix m) {
    return m == v4l2::ColorMatrix::Bt709 ? dmabuf_header::ColorMatrix::Bt709
                                         : dmabuf_header::ColorMatrix::Bt601;
}

int broadcast_nv12(scm_rights_producer::ScmRightsProducer& prod, const jpeg_dec::DecodedNv12& d,
                   uint64_t frame_idx,
                   dmabuf_header::ColorMatrix matrix = dmabuf_header::ColorMatrix::Bt601);

void broadcast_buffer(scm_rights_producer::ScmRightsProducer& prod, const nv12_buf::Buffer& b,
                      uint64_t frame_idx,
                      dmabuf_header::ColorMatrix matrix = dmabuf_header::ColorMatrix::Bt601);

} // namespace source

// Forward decl avoids dragging the heavy grpc + protobuf headers into
// every includer of broadcast.hpp. The orchestrator includes the proto
// header directly when it needs to actually pass a Status.
namespace videonode::control {
class Status;
}

namespace source {

struct StatusContext {
    const std::string& device_id;
    source_probe::SourceProbe& probe;
    source_probe::Health health;
    const CaptureSession& cap;
    const Args& args;
    uint64_t real_frame_idx;
    uint64_t placeholder_frames;
    uint32_t last_seq;
    scm_rights_producer::ScmRightsProducer& prod;
};

void build_status_proto(::videonode::control::Status& out, const StatusContext& ctx);

} // namespace source
