// broadcast — SCM_RIGHTS publish helpers + status-snapshot serializer.
// Internal header for the source/ library.
#pragma once

#include "src/capture/jpeg_dec.hpp"
#include "src/capture/source_probe.hpp"
#include "src/ipc/scm_rights_producer.hpp"
#include "src/render/nv12_buf.hpp"
#include "src/source/args.hpp"
#include "src/source/capture_session.hpp"

#include <cstdint>
#include <string>

namespace source {

// Monotonic milliseconds since steady_clock epoch.
uint64_t now_ms();

// Send a decoded NV12 frame to all connected SCM consumers.
void broadcast_nv12(scm_rights_producer::ScmRightsProducer& prod, const jpeg_dec::DecodedNv12& d,
                    uint64_t frame_idx);

// Thin shim for placeholder + transitioning re-broadcast paths. Reads
// layout straight from the nv12_buf::Buffer so split-buffer and single-
// buffer backends both work.
void broadcast_buffer(scm_rights_producer::ScmRightsProducer& prod, const nv12_buf::Buffer& b,
                      uint64_t frame_idx);

} // namespace source

// Forward decl avoids dragging the heavy grpc + protobuf headers into
// every includer of broadcast.hpp. The orchestrator includes the proto
// header directly when it needs to actually pass a Status.
namespace videonode::control {
class Status;
}

namespace source {

// Populate a Status proto from the current capture / probe / broadcast
// state. Called by the orchestrator on health change, consumer-count
// change, or once per second as a heartbeat; the gRPC StreamStatus
// subscribers receive each published Status.
void build_status_proto(::videonode::control::Status& out, const std::string& device_id,
                        source_probe::SourceProbe& probe, source_probe::Health h,
                        const CaptureSession& cap, const Args& a, uint64_t real_frame_idx,
                        uint64_t placeholder_frames, uint32_t last_seq,
                        scm_rights_producer::ScmRightsProducer& prod);

} // namespace source
