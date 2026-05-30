// broadcast — SCM_RIGHTS publish helpers + status-snapshot serializer.
// Internal header for the source/ library.
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

// Build a snapshot FrameRef from a decoded NV12 frame (or a raw NV12
// buffer). Carries the dma-buf fds + plane geometry + slot/generation so
// the holder can pin the ring slot during a Snapshot read.
vn::snapshot::FrameRef make_frame_ref(const jpeg_dec::DecodedNv12& d, uint64_t frame_idx);
vn::snapshot::FrameRef make_frame_ref(const nv12_buf::Buffer& b, uint64_t frame_idx);

// Monotonic milliseconds since steady_clock epoch.
uint64_t now_ms();

// Wall-clock milliseconds since the Unix epoch (system_clock). Used for
// the status timestamp so the UI can render a real date; never for
// interval math (use now_ms() — it is immune to clock jumps).
int64_t wall_ms();

// Send a decoded NV12 frame to all connected SCM consumers. Returns the
// number of consumers it was delivered to, for SlotOwner refcounting.
int broadcast_nv12(scm_rights_producer::ScmRightsProducer& prod, const jpeg_dec::DecodedNv12& d,
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

// Parameters for build_status_proto. Groups the 10 individual arguments so
// callers don't have to maintain a long parameter list at every call site.
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

// Populate a Status proto from the current capture / probe / broadcast
// state. Called by the orchestrator on health change, consumer-count
// change, or once per second as a heartbeat; the gRPC StreamStatus
// subscribers receive each published Status.
void build_status_proto(::videonode::control::Status& out, const StatusContext& ctx);

} // namespace source
