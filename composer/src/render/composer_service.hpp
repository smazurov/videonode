// composer_service — gRPC service implementation for videonode-composer.
//
// Implements videonode.control.Composer. The daemon dials in over the
// per-instance UDS (passed via --grpc-listen), calls Describe() first
// to record identity, then issues unary RPCs that mutate the composer's
// World snapshot. Each handler translates the proto request into the
// existing composer_rpc strong-typed struct and forwards to World's
// apply_* methods so the render loop keeps consuming the same Snapshot
// shape it always did.

#pragma once

#include "control/composer.grpc.pb.h"

#include <atomic>
#include <string>

namespace render {
class World;
}

namespace nativerpc {

struct ComposerContext {
    render::World* world = nullptr;         // mutated by handlers
    std::atomic<bool>* running = nullptr;   // flipped false on Shutdown
    std::string composer_id;                // returned by Describe
    std::string version;                    // returned by Describe
};

class ComposerService final : public videonode::control::Composer::Service {
  public:
    explicit ComposerService(ComposerContext ctx);

    grpc::Status Describe(grpc::ServerContext* ctx,
                          const ::google::protobuf::Empty* req,
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

    grpc::Status Shutdown(grpc::ServerContext* ctx,
                          const ::google::protobuf::Empty* req,
                          ::google::protobuf::Empty* resp) override;

  private:
    ComposerContext ctx_;
};

} // namespace nativerpc
