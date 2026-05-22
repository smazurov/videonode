// source_service — gRPC service implementation for videonode-source.
//
// Implements videonode.control.Source. The daemon dials in over the
// per-instance UDS (passed via --grpc-listen) and:
//   - calls Describe() to record identity (replaces the legacy `identify`
//     JSON-RPC notification),
//   - issues unary SetFormat / GetStatus / Snapshot / Shutdown control
//     RPCs,
//   - opens a long-lived StreamStatus server-stream that yields a Status
//     proto on every health/consumer/heartbeat change.
//
// The orchestrator owns the actual capture/broadcast state. It publishes
// to this service via two producer-thread entry points:
//   - PublishStatus(...) — call whenever a status notification would have
//     fired under the legacy ctl.push_status() path.
//   - UpdateLastFrame(...) — call after each NV12 frame is produced so
//     Snapshot() can return without blocking the broadcast loop.

#pragma once

#include "control/source.grpc.pb.h"

#include <atomic>
#include <condition_variable>
#include <cstdint>
#include <mutex>
#include <optional>
#include <string>
#include <vector>

// Forward decls — service code calls through pointers, orchestrator owns
// the concrete types. This keeps the header free of source/ internals.
namespace source {
struct Args;
}
namespace source_probe {
class SourceProbe;
}

namespace nativerpc {

// LatestFrame is an in-process copy of the most-recently-produced NV12
// frame. The broadcast loop calls UpdateLastFrame() after each frame so
// Snapshot() can return without touching dma-buf or blocking the
// producer. ~3 MB per 1080p frame; one allocation owned, reused on the
// orchestrator side.
struct LatestFrame {
    std::vector<uint8_t> nv12;
    uint32_t width = 0;
    uint32_t height = 0;
    uint32_t pitch_y = 0;
    uint32_t pitch_uv = 0;
    uint64_t frame_idx = 0;
    uint64_t captured_at_ns = 0; // CLOCK_MONOTONIC nanoseconds
};

struct SourceContext {
    std::string device_id;
    std::string version;
    std::atomic<bool>* running = nullptr; // flipped false on Shutdown

    // SetFormat — orchestrator owns Args + need_reinit; the service
    // mutates them under set_format_mu and the orchestrator reads them
    // at the next loop iteration.
    std::mutex set_format_mu;
    source::Args* args = nullptr;
    bool* need_reinit_for_format_change = nullptr;
    source_probe::SourceProbe* probe = nullptr;
};

class SourceService final : public videonode::control::Source::Service {
  public:
    explicit SourceService(SourceContext* ctx);

    // gRPC handlers ------------------------------------------------------

    grpc::Status Describe(grpc::ServerContext* ctx,
                          const ::google::protobuf::Empty* req,
                          ::videonode::control::NativeInfo* resp) override;

    grpc::Status SetFormat(grpc::ServerContext* ctx,
                           const ::videonode::control::SetFormatRequest* req,
                           ::videonode::control::SetFormatResponse* resp) override;

    grpc::Status GetStatus(grpc::ServerContext* ctx,
                           const ::google::protobuf::Empty* req,
                           ::videonode::control::Status* resp) override;

    grpc::Status StreamStatus(grpc::ServerContext* ctx,
                              const ::google::protobuf::Empty* req,
                              grpc::ServerWriter<::videonode::control::Status>* writer) override;

    grpc::Status Snapshot(grpc::ServerContext* ctx,
                          const ::videonode::control::SnapshotRequest* req,
                          ::videonode::control::SnapshotResponse* resp) override;

    grpc::Status Shutdown(grpc::ServerContext* ctx,
                          const ::google::protobuf::Empty* req,
                          ::google::protobuf::Empty* resp) override;

    // Producer-thread entry points --------------------------------------

    // Publish a new Status snapshot. Wakes any active StreamStatus RPCs;
    // they each receive the most-recent snapshot on the next iteration.
    void PublishStatus(const ::videonode::control::Status& s);

    // Stash the most-recent NV12 frame for Snapshot() to return. Moves
    // the LatestFrame into the holder; subsequent updates overwrite the
    // previous one (last-write-wins).
    void UpdateLastFrame(LatestFrame f);

    // Tell every active StreamStatus to flush and return. Called from
    // the orchestrator's shutdown path so the server thread can join.
    //
    // SourceService is single-use: once StopStreams() has been called
    // the service is unusable (stop_streams_ stays true). The binary
    // exits shortly after, so this is the expected lifecycle. If a
    // future in-process restart path needs to reuse a SourceService
    // across Stop/Start cycles, add a Reset() that clears
    // stop_streams_ before reopening the gRPC server.
    void StopStreams();

  private:
    SourceContext* ctx_;

    std::mutex status_mu_;
    std::condition_variable status_cv_;
    std::optional<::videonode::control::Status> last_status_;
    uint64_t status_version_ = 0;
    bool stop_streams_ = false;

    std::mutex frame_mu_;
    std::optional<LatestFrame> last_frame_;
};

} // namespace nativerpc
