#include "src/render/composer_service.hpp"

#include "src/render/canvas_loop.hpp"
#include "src/render/world.hpp"
#include "src/rpc/composer_rpc.hpp"

#include <unistd.h>

namespace nativerpc {

namespace {

// Map a composer_rpc::ParseError to a grpc::Status. The parsers emit
// JSON-RPC codes (-32602 invalid params, -32000 semantic reject); the
// gRPC equivalents are INVALID_ARGUMENT for both since the daemon
// doesn't disambiguate today.
grpc::Status from_parse_error(const composer_rpc::ParseError& e) {
    return grpc::Status(grpc::StatusCode::INVALID_ARGUMENT, e.message);
}

} // namespace

ComposerService::ComposerService(ComposerContext ctx) : ctx_(std::move(ctx)) {}

grpc::Status ComposerService::Describe(grpc::ServerContext* /*ctx*/,
                                       const ::google::protobuf::Empty* /*req*/,
                                       ::videonode::control::NativeInfo* resp) {
    resp->set_device_id(ctx_.composer_id);
    resp->set_pid(static_cast<uint32_t>(::getpid()));
    resp->set_version(ctx_.version);
    resp->set_kind("composer");
    resp->set_protocol_version(1);
    return grpc::Status::OK;
}

grpc::Status ComposerService::SetCanvas(grpc::ServerContext* /*ctx*/,
                                        const ::videonode::control::SetCanvasRequest* req,
                                        ::google::protobuf::Empty* /*resp*/) {
    composer_rpc::SetCanvasRequest r;
    r.w = req->w();
    r.h = req->h();
    r.fps = req->fps();
    composer_rpc::ParseError e;
    if (!ctx_.world->apply_set_canvas(r, e)) {
        return from_parse_error(e);
    }
    return grpc::Status::OK;
}

grpc::Status ComposerService::SetSource(grpc::ServerContext* /*ctx*/,
                                        const ::videonode::control::SetSourceRequest* req,
                                        ::google::protobuf::Empty* /*resp*/) {
    composer_rpc::SetSourceRequest r;
    r.slot = req->slot();
    r.source_id = req->source_id();
    r.scm_path = req->scm_path();
    r.width = req->width();
    r.height = req->height();
    r.fps = req->fps();
    composer_rpc::ParseError e;
    if (!ctx_.world->apply_set_source(r, e)) {
        return from_parse_error(e);
    }
    return grpc::Status::OK;
}

grpc::Status ComposerService::ClearSource(grpc::ServerContext* /*ctx*/,
                                          const ::videonode::control::ClearSourceRequest* req,
                                          ::google::protobuf::Empty* /*resp*/) {
    composer_rpc::ClearSourceRequest r;
    r.slot = req->slot();
    composer_rpc::ParseError e;
    if (!ctx_.world->apply_clear_source(r, e)) {
        return from_parse_error(e);
    }
    return grpc::Status::OK;
}

grpc::Status ComposerService::SetLayout(grpc::ServerContext* /*ctx*/,
                                        const ::videonode::control::SetLayoutRequest* req,
                                        ::google::protobuf::Empty* /*resp*/) {
    composer_rpc::SetLayoutRequest r;
    r.slots.reserve(static_cast<size_t>(req->slots_size()));
    for (const auto& s : req->slots()) {
        composer_rpc::LayoutSlot ls;
        ls.slot = s.slot();
        ls.x = s.x();
        ls.y = s.y();
        ls.w = s.w();
        ls.h = s.h();
        r.slots.push_back(std::move(ls));
    }
    composer_rpc::ParseError e;
    if (!ctx_.world->apply_set_layout(r, e)) {
        return from_parse_error(e);
    }
    return grpc::Status::OK;
}

grpc::Status ComposerService::SetEffects(grpc::ServerContext* /*ctx*/,
                                         const ::videonode::control::SetEffectsRequest* req,
                                         ::google::protobuf::Empty* /*resp*/) {
    composer_rpc::SetEffectsRequest r;
    r.source_id = req->source_id();
    r.effects.reserve(static_cast<size_t>(req->effects_size()));
    for (const auto& ef : req->effects()) {
        composer_rpc::Effect out;
        out.type = ef.type();
        out.recognized = (out.type == "perspective");
        if (out.recognized && ef.has_perspective()) {
            const auto& p = ef.perspective();
            composer_rpc::PerspectiveEffectParams pp;
            // corners is a flat repeated int32 of length 8 by contract
            // (TLx,TLy,TRx,TRy,BRx,BRy,BLx,BLy). The parser rejects
            // anything else; mirror that here.
            if (p.corners_size() != 8) {
                composer_rpc::ParseError e;
                e.code = -32602;
                e.message = "perspective.corners must have exactly 8 elements";
                return from_parse_error(e);
            }
            for (int i = 0; i < 8; ++i) {
                pp.corners[i] = p.corners(i);
            }
            pp.snapshot_w = p.snapshot_w();
            pp.snapshot_h = p.snapshot_h();
            out.perspective = pp;
        }
        r.effects.push_back(std::move(out));
    }
    composer_rpc::ParseError e;
    if (!ctx_.world->apply_set_effects(r, e)) {
        return from_parse_error(e);
    }
    return grpc::Status::OK;
}

grpc::Status ComposerService::SetSourceState(grpc::ServerContext* /*ctx*/,
                                             const ::videonode::control::SetSourceStateRequest* req,
                                             ::google::protobuf::Empty* /*resp*/) {
    composer_rpc::SetSourceStateRequest r;
    r.source_id = req->source_id();
    r.state = req->state();
    composer_rpc::ParseError e;
    if (!ctx_.world->apply_set_source_state(r, e)) {
        return from_parse_error(e);
    }
    return grpc::Status::OK;
}

grpc::Status ComposerService::GetStats(grpc::ServerContext* /*ctx*/,
                                       const ::google::protobuf::Empty* /*req*/,
                                       ::videonode::control::ComposerStats* resp) {
    if (!ctx_.stats) {
        return grpc::Status(grpc::StatusCode::FAILED_PRECONDITION,
                            "render stats not attached to ComposerContext");
    }
    const auto& s = *ctx_.stats;
    resp->set_frames_rendered(s.frames_rendered.load(std::memory_order_relaxed));
    resp->set_fps_observed(s.fps_observed_centi.load(std::memory_order_relaxed) / 100.0);
    resp->set_canvas_w(s.canvas_w.load(std::memory_order_relaxed));
    resp->set_canvas_h(s.canvas_h.load(std::memory_order_relaxed));
    resp->set_canvas_fps(s.canvas_fps.load(std::memory_order_relaxed));
    resp->set_consumer_count(s.consumer_count.load(std::memory_order_relaxed));
    return grpc::Status::OK;
}

grpc::Status ComposerService::Shutdown(grpc::ServerContext* /*ctx*/,
                                       const ::google::protobuf::Empty* /*req*/,
                                       ::google::protobuf::Empty* /*resp*/) {
    if (ctx_.running) {
        ctx_.running->store(false);
    }
    return grpc::Status::OK;
}

} // namespace nativerpc
