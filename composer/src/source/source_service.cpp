#include "src/source/source_service.hpp"

#include "src/capture/source_probe.hpp"
#include "src/common/log_levels.hpp"
#include "src/source/args.hpp"
#include "src/source/capture_session.hpp" // for v4l2_pix_fmt_

#include <chrono>
#include <unistd.h>

namespace nativerpc {

namespace {

// Heartbeat tick for StreamStatus when the producer is quiet: wake up
// every second so the daemon's keepalive can detect a half-dead stream
// even if no status events have fired.
constexpr auto kStreamStatusIdleTick = std::chrono::seconds(1);

} // namespace

SourceService::SourceService(SourceContext* ctx) : ctx_(ctx) {}

grpc::Status SourceService::Describe(grpc::ServerContext* /*ctx*/,
                                     const ::google::protobuf::Empty* /*req*/,
                                     ::videonode::control::NativeInfo* resp) {
    resp->set_device_id(ctx_->device_id);
    resp->set_pid(static_cast<uint32_t>(::getpid()));
    resp->set_version(ctx_->version);
    resp->set_kind("source");
    resp->set_protocol_version(1);
    return grpc::Status::OK;
}

grpc::Status SourceService::SetFormat(grpc::ServerContext* /*ctx*/,
                                      const ::videonode::control::SetFormatRequest* req,
                                      ::videonode::control::SetFormatResponse* resp) {
    if (source::v4l2_pix_fmt_(req->fourcc()) == 0) {
        return grpc::Status(grpc::StatusCode::INVALID_ARGUMENT, "unsupported fourcc");
    }
    // Mirror the validation the deleted set_format_parser performed.
    if (req->w() == 0 || req->h() == 0) {
        return grpc::Status(grpc::StatusCode::INVALID_ARGUMENT, "w/h must be > 0");
    }
    if (req->w() > 16384 || req->h() > 16384) {
        return grpc::Status(grpc::StatusCode::INVALID_ARGUMENT, "w/h exceed 16384");
    }
    // NV12 / NV21 / NV24 / YUYV / UYVY all require even width; YUV
    // 4:2:0 family additionally requires even height. The deleted
    // parser enforced even-w/h universally — keep that contract.
    if ((req->w() & 1u) != 0 || (req->h() & 1u) != 0) {
        return grpc::Status(grpc::StatusCode::INVALID_ARGUMENT, "w/h must be even");
    }
    if (req->fps() > 240) {
        return grpc::Status(grpc::StatusCode::INVALID_ARGUMENT, "fps exceeds 240");
    }
    {
        std::lock_guard<std::mutex> lock(ctx_->set_format_mu);
        if (ctx_->args) {
            ctx_->args->in_format = req->fourcc();
            ctx_->args->in_width = static_cast<int>(req->w());
            ctx_->args->in_height = static_cast<int>(req->h());
            ctx_->args->in_fps = static_cast<int>(req->fps());
        }
        if (ctx_->probe) {
            ctx_->probe->note_format_change();
        }
        if (ctx_->need_reinit_for_format_change) {
            *ctx_->need_reinit_for_format_change = true;
        }
    }
    resp->set_applied(true);
    vn::log::info("videonode-source: set_format via gRPC: %s %ux%u@%u",
                  req->fourcc().c_str(), req->w(), req->h(), req->fps());
    return grpc::Status::OK;
}

grpc::Status SourceService::GetStatus(grpc::ServerContext* /*ctx*/,
                                      const ::google::protobuf::Empty* /*req*/,
                                      ::videonode::control::Status* resp) {
    std::lock_guard<std::mutex> lock(status_mu_);
    if (!last_status_) {
        return grpc::Status(grpc::StatusCode::UNAVAILABLE, "no status snapshot yet");
    }
    *resp = *last_status_;
    return grpc::Status::OK;
}

grpc::Status SourceService::StreamStatus(grpc::ServerContext* ctx,
                                         const ::google::protobuf::Empty* /*req*/,
                                         grpc::ServerWriter<::videonode::control::Status>* writer) {
    uint64_t last_seen = 0;
    while (true) {
        ::videonode::control::Status to_send;
        bool have = false;
        {
            std::unique_lock<std::mutex> lock(status_mu_);
            status_cv_.wait_for(lock, kStreamStatusIdleTick, [&] {
                return stop_streams_ || ctx->IsCancelled() ||
                       (last_status_.has_value() && status_version_ != last_seen);
            });
            if (stop_streams_ || ctx->IsCancelled()) {
                return grpc::Status::OK;
            }
            if (last_status_ && status_version_ != last_seen) {
                to_send = *last_status_;
                last_seen = status_version_;
                have = true;
            }
        }
        if (have) {
            if (!writer->Write(to_send)) {
                // Client went away.
                return grpc::Status::OK;
            }
        }
    }
}

grpc::Status SourceService::Snapshot(grpc::ServerContext* /*ctx*/,
                                     const ::videonode::control::SnapshotRequest* /*req*/,
                                     ::videonode::control::SnapshotResponse* resp) {
    std::lock_guard<std::mutex> lock(frame_mu_);
    if (!last_frame_) {
        return grpc::Status(grpc::StatusCode::UNAVAILABLE, "no frame produced yet");
    }
    const auto& f = *last_frame_;
    resp->set_nv12(f.nv12.data(), f.nv12.size());
    resp->set_width(f.width);
    resp->set_height(f.height);
    resp->set_pitch_y(f.pitch_y);
    resp->set_pitch_uv(f.pitch_uv);
    resp->set_frame_idx(f.frame_idx);
    resp->set_captured_at_ns(f.captured_at_ns);
    return grpc::Status::OK;
}

grpc::Status SourceService::Shutdown(grpc::ServerContext* /*ctx*/,
                                     const ::google::protobuf::Empty* /*req*/,
                                     ::google::protobuf::Empty* /*resp*/) {
    if (ctx_->running) {
        ctx_->running->store(false);
    }
    StopStreams();
    return grpc::Status::OK;
}

void SourceService::PublishStatus(const ::videonode::control::Status& s) {
    {
        std::lock_guard<std::mutex> lock(status_mu_);
        last_status_ = s;
        ++status_version_;
    }
    status_cv_.notify_all();
}

void SourceService::UpdateLastFrame(LatestFrame f) {
    std::lock_guard<std::mutex> lock(frame_mu_);
    last_frame_ = std::move(f);
}

void SourceService::StopStreams() {
    {
        std::lock_guard<std::mutex> lock(status_mu_);
        stop_streams_ = true;
    }
    status_cv_.notify_all();
}

} // namespace nativerpc
