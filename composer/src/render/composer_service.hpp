#pragma once

#include "control/composer.grpc.pb.h"
#include "src/snapshot/snapshot.hpp"

#include <atomic>
#include <condition_variable>
#include <mutex>
#include <string>

namespace render {
class World;
struct RenderStats;
} // namespace render

namespace nativerpc {

struct ComposerContext {
    render::World* world = nullptr;
    std::atomic<bool>* running = nullptr; // flipped false on Shutdown
    render::RenderStats* stats = nullptr;
    std::string composer_id;
    std::string version;
};

class ComposerService final : public videonode::control::Composer::Service {
  public:
    explicit ComposerService(ComposerContext ctx);

    grpc::Status Describe(grpc::ServerContext* ctx, const ::google::protobuf::Empty* req,
                          ::videonode::control::NativeInfo* resp) override;

    grpc::Status SetCanvas(grpc::ServerContext* ctx,
                           const ::videonode::control::SetCanvasRequest* req,
                           ::google::protobuf::Empty* resp) override;

    grpc::Status SetSource(grpc::ServerContext* ctx,
                           const ::videonode::control::SetSourceRequest* req,
                           ::google::protobuf::Empty* resp) override;

    grpc::Status ClearSource(grpc::ServerContext* ctx,
                             const ::videonode::control::ClearSourceRequest* req,
                             ::google::protobuf::Empty* resp) override;

    grpc::Status SetLayout(grpc::ServerContext* ctx,
                           const ::videonode::control::SetLayoutRequest* req,
                           ::google::protobuf::Empty* resp) override;

    grpc::Status SetEffects(grpc::ServerContext* ctx,
                            const ::videonode::control::SetEffectsRequest* req,
                            ::google::protobuf::Empty* resp) override;

    grpc::Status SetSourceState(grpc::ServerContext* ctx,
                                const ::videonode::control::SetSourceStateRequest* req,
                                ::google::protobuf::Empty* resp) override;

    grpc::Status GetStats(grpc::ServerContext* ctx, const ::google::protobuf::Empty* req,
                          ::videonode::control::ComposerStats* resp) override;

    grpc::Status Snapshot(grpc::ServerContext* ctx,
                          const ::videonode::control::ComposerSnapshotRequest* req,
                          ::videonode::control::ComposerSnapshotResponse* resp) override;

    grpc::Status Shutdown(grpc::ServerContext* ctx, const ::google::protobuf::Empty* req,
                          ::google::protobuf::Empty* resp) override;

    void UpdateLatestCanvas(vn::snapshot::FrameRef ref);

    // True when a Snapshot() RPC is waiting; the canvas loop fills one frame
    // on demand instead of copying every frame.
    [[nodiscard]] bool snapshot_pending() const {
        return snapshot_requested_.load(std::memory_order_acquire);
    }

  private:
    ComposerContext ctx_;
    vn::snapshot::LatestFrameHolder frame_holder_;

    std::atomic<bool> snapshot_requested_{false};
    mutable std::mutex snap_mu_;
    std::condition_variable snap_cv_;
    uint64_t snap_seq_ = 0;
};

} // namespace nativerpc
