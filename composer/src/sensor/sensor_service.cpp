#include "src/sensor/sensor_service.hpp"

#include "version.hpp"

#include <unistd.h>

namespace sensor {

namespace {
constexpr size_t kMaxQueued = 16;
constexpr uint32_t kProtocolVersion = 1;
} // namespace

SensorService::SensorService(SensorContext* ctx) : ctx_(ctx) {}

grpc::Status SensorService::Describe(grpc::ServerContext* /*ctx*/,
                                     const ::google::protobuf::Empty* /*req*/,
                                     ::videonode::control::SensorInfo* resp) {
    auto* native = resp->mutable_native();
    native->set_device_id(ctx_->sensor_id);
    native->set_pid(static_cast<uint32_t>(::getpid()));
    native->set_version(ctx_->version.empty() ? vn::kVersion : ctx_->version);
    native->set_kind("sensor");
    native->set_protocol_version(kProtocolVersion);
    resp->set_schema_version(ctx_->schema_version);
    resp->set_model_id(ctx_->model_id);
    resp->add_kinds("bbox");
    return grpc::Status::OK;
}

grpc::Status SensorService::Configure(grpc::ServerContext* /*ctx*/,
                                      const ::videonode::control::ConfigureRequest* req,
                                      ::google::protobuf::Empty* /*resp*/) {
    std::lock_guard<std::mutex> g(mu_);
    if (!req->target_ref().empty())
        ctx_->target_ref = req->target_ref();
    if (!req->mode().empty())
        mode_ = req->mode();
    return grpc::Status::OK;
}

grpc::Status SensorService::AnalyzeOnce(grpc::ServerContext* /*ctx*/,
                                        const ::videonode::control::AnalyzeRequest* /*req*/,
                                        ::videonode::control::Finding* resp) {
    std::lock_guard<std::mutex> g(mu_);
    if (!last_)
        return {grpc::StatusCode::UNAVAILABLE, "no finding produced yet"};
    *resp = *last_;
    return grpc::Status::OK;
}

grpc::Status
SensorService::StreamFindings(grpc::ServerContext* sctx,
                              const ::videonode::control::StreamFindingsRequest* /*req*/,
                              grpc::ServerWriter<::videonode::control::Finding>* writer) {
    std::unique_lock<std::mutex> lk(mu_);
    for (;;) {
        cv_.wait(lk, [&] { return stop_ || !queue_.empty() || sctx->IsCancelled(); });
        if (stop_ || sctx->IsCancelled())
            return grpc::Status::OK;
        while (!queue_.empty()) {
            ::videonode::control::Finding f = std::move(queue_.front());
            queue_.pop_front();
            lk.unlock();
            if (!writer->Write(f)) {
                lk.lock();
                return grpc::Status::OK;
            }
            lk.lock();
        }
    }
}

grpc::Status SensorService::Shutdown(grpc::ServerContext* /*ctx*/,
                                     const ::google::protobuf::Empty* /*req*/,
                                     ::google::protobuf::Empty* /*resp*/) {
    if (ctx_->running != nullptr)
        ctx_->running->store(false);
    StopStreams();
    return grpc::Status::OK;
}

void SensorService::Publish(const ::videonode::control::Finding& f) {
    std::lock_guard<std::mutex> g(mu_);
    last_ = f;
    queue_.push_back(f);
    while (queue_.size() > kMaxQueued)
        queue_.pop_front();
    cv_.notify_all();
}

std::string SensorService::mode() {
    std::lock_guard<std::mutex> g(mu_);
    return mode_;
}

void SensorService::StopStreams() {
    std::lock_guard<std::mutex> g(mu_);
    stop_ = true;
    cv_.notify_all();
}

} // namespace sensor
