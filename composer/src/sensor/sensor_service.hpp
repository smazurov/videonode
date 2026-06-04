#pragma once

#include "control/sensor.grpc.pb.h"

#include <atomic>
#include <condition_variable>
#include <cstdint>
#include <deque>
#include <mutex>
#include <optional>
#include <string>

namespace sensor {

struct SensorContext {
    std::string sensor_id;
    std::string version;
    std::string model_id;
    std::string target_ref; // seeded from --target-ref; may be retuned by Configure
    uint32_t schema_version = 1;
    std::atomic<bool>* running = nullptr; // flipped false on Shutdown
};

// SensorService is the daemon-facing gRPC surface. The orchestrator
// produces Findings via Publish(); StreamFindings drains a bounded queue to
// the (single) daemon subscriber. The contract is perception-only — no action
// RPCs.
class SensorService final : public videonode::control::Sensor::Service {
  public:
    explicit SensorService(SensorContext* ctx);

    grpc::Status Describe(grpc::ServerContext* ctx, const ::google::protobuf::Empty* req,
                          ::videonode::control::SensorInfo* resp) override;

    grpc::Status Configure(grpc::ServerContext* ctx,
                           const ::videonode::control::ConfigureRequest* req,
                           ::google::protobuf::Empty* resp) override;

    grpc::Status AnalyzeOnce(grpc::ServerContext* ctx,
                             const ::videonode::control::AnalyzeRequest* req,
                             ::videonode::control::Finding* resp) override;

    grpc::Status StreamFindings(grpc::ServerContext* ctx,
                                const ::videonode::control::StreamFindingsRequest* req,
                                grpc::ServerWriter<::videonode::control::Finding>* writer) override;

    grpc::Status Shutdown(grpc::ServerContext* ctx, const ::google::protobuf::Empty* req,
                          ::google::protobuf::Empty* resp) override;

    // Called from the orchestrator thread. Stamps nothing — the caller fills
    // the envelope. Bounded: oldest Findings are dropped under backpressure.
    void Publish(const ::videonode::control::Finding& f);

    // Current operator-selected mode ("propose" | "auto"); default "auto".
    [[nodiscard]] std::string mode();

    void StopStreams();

  private:
    SensorContext* ctx_;

    std::mutex mu_;
    std::condition_variable cv_;
    std::deque<::videonode::control::Finding> queue_;
    std::optional<::videonode::control::Finding> last_;
    std::string mode_ = "auto";
    bool stop_ = false;
};

} // namespace sensor
