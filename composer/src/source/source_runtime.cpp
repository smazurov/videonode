#include "src/source/source_runtime.hpp"

#include "src/common/log_levels.hpp"
#include "version.hpp"
#if defined(HAVE_GBM) && !defined(HAVE_RGA)
#include "src/render/csc_placebo.hpp"
#endif

#include <vector>

namespace source {

// rig (HAVE_RGA): dma_heap-backed; host: must share csc_placebo's GBM device.
bool init_allocator(nv12_buf::Allocator& allocator) {
#if defined(HAVE_GBM) && !defined(HAVE_RGA)
    if (!csc_placebo::init()) {
        vn::log::fatal(
            "videonode-source: csc_placebo::init failed; cannot bring up Mesa CSC backend "
            "(needed for the GBM allocator's gbm_device)");
        return false;
    }
    gbm_device* alloc_gbm = csc_placebo::gbm_device_for_io();
    if (alloc_gbm == nullptr) {
        vn::log::fatal("videonode-source: csc_placebo::gbm_device_for_io returned null");
        return false;
    }
    if (!allocator.init(alloc_gbm)) {
        vn::log::fatal("videonode-source: nv12_buf::Allocator::init failed");
        return false;
    }
#else
    if (!allocator.init()) {
        vn::log::fatal("videonode-source: nv12_buf::Allocator::init failed");
        return false;
    }
#endif
    return true;
}

bool start_grpc(const Args& a, nativerpc::SourceService& grpc_svc, nativerpc::GrpcServer& grpc_srv,
                bool& grpc_enabled) {
    grpc_enabled = !a.grpc_listen.empty() && !a.device_id.empty();
    if (!grpc_enabled)
        return true;
    std::vector<grpc::Service*> services = {&grpc_svc};
    if (!grpc_srv.Start(a.grpc_listen, services)) {
        vn::log::fatal("videonode-source: gRPC server failed to start on %s",
                       a.grpc_listen.c_str());
        grpc_enabled = false;
        return false;
    }
    vn::log::debug("videonode-source: grpc server listening on %s (id=%s)", a.grpc_listen.c_str(),
                   a.device_id.c_str());
    return true;
}

void populate_gctx(nativerpc::SourceContext& gctx, std::atomic<bool>& running, Args& a,
                   bool& need_reinit_flag, source_probe::SourceProbe& probe,
                   std::optional<nativerpc::ActiveFormat>& active_format) {
    gctx.device_id = a.device_id;
    gctx.version = vn::kVersion;
    gctx.running = &running;
    gctx.args = &a;
    gctx.need_reinit_for_format_change = &need_reinit_flag;
    gctx.probe = &probe;
    gctx.active_format = &active_format;
}

} // namespace source
