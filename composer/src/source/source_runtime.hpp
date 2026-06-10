#pragma once

#include "src/render/nv12_buf.hpp"
#include "src/rpc/grpc_server.hpp"
#include "src/source/args.hpp"
#include "src/source/source_service.hpp"

#include <atomic>
#include <optional>

namespace source_probe {
class SourceProbe;
}

namespace source {

[[nodiscard]] bool init_allocator(nv12_buf::Allocator& allocator);

[[nodiscard]] bool start_grpc(const Args& a, nativerpc::SourceService& grpc_svc,
                              nativerpc::GrpcServer& grpc_srv, bool& grpc_enabled);

void populate_gctx(nativerpc::SourceContext& gctx, std::atomic<bool>& running, Args& a,
                   bool& need_reinit_flag, source_probe::SourceProbe& probe,
                   std::optional<nativerpc::ActiveFormat>& active_format);

} // namespace source
