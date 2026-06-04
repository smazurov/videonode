#pragma once

#include "control/source.grpc.pb.h"
#include "src/snapshot/snapshot.hpp"

#include <atomic>
#include <condition_variable>
#include <cstdint>
#include <memory>
#include <mutex>
#include <optional>
#include <string>

namespace source {
struct Args;
}
namespace source_probe {
class SourceProbe;
}

namespace nativerpc {

struct ActiveFormat {
    std::string fourcc;
    uint32_t w = 0;
    uint32_t h = 0;
    // 0 = "driver decides" — we never pinned a rate. Treated as a wildcard
    // in SetFormat's match check; a non-zero requested fps against an
    // unpinned active fps does not force a rebuild.
    uint32_t fps = 0;
};

struct SourceContext {
    std::string device_id;
    std::string version;
    std::atomic<bool>* running = nullptr; // flipped false on Shutdown

    std::mutex set_format_mu;
    source::Args* args = nullptr;
    bool* need_reinit_for_format_change = nullptr;
    source_probe::SourceProbe* probe = nullptr;
    // Engaged once try_open_capture succeeds; cleared on teardown. Read
    // under set_format_mu so SetFormat can no-op when the request matches.
    std::optional<ActiveFormat>* active_format = nullptr;
};

class SourceService final : public videonode::control::Source::Service {
  public:
    explicit SourceService(SourceContext* ctx);

    grpc::Status Describe(grpc::ServerContext* ctx, const ::google::protobuf::Empty* req,
                          ::videonode::control::NativeInfo* resp) override;

    grpc::Status SetFormat(grpc::ServerContext* ctx,
                           const ::videonode::control::SetFormatRequest* req,
                           ::videonode::control::SetFormatResponse* resp) override;

    grpc::Status SetDevice(grpc::ServerContext* ctx,
                           const ::videonode::control::SetDeviceRequest* req,
                           ::videonode::control::SetDeviceResponse* resp) override;

    grpc::Status GetStatus(grpc::ServerContext* ctx, const ::google::protobuf::Empty* req,
                           ::videonode::control::Status* resp) override;

    grpc::Status StreamStatus(grpc::ServerContext* ctx, const ::google::protobuf::Empty* req,
                              grpc::ServerWriter<::videonode::control::Status>* writer) override;

    grpc::Status Snapshot(grpc::ServerContext* ctx,
                          const ::videonode::control::SnapshotRequest* req,
                          ::videonode::control::SnapshotResponse* resp) override;

    grpc::Status Shutdown(grpc::ServerContext* ctx, const ::google::protobuf::Empty* req,
                          ::google::protobuf::Empty* resp) override;

    void PublishStatus(const ::videonode::control::Status& s);

    // Cheap — just copies the FrameRef under a mutex. Snapshot() does the mmap+pack
    // lazily so the broadcast loop never pays for unused snapshots.
    void UpdateLastFrame(vn::snapshot::FrameRef ref);

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

    vn::snapshot::LatestFrameHolder frame_holder_;
};

} // namespace nativerpc
