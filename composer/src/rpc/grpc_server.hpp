// The native binaries run their gRPC server on a per-instance UDS that
// the daemon allocates and passes via --grpc-listen. When --grpc-listen
// is empty the binary skips the server entirely and runs in standalone
// mode (R-smoke scenarios depend on this).

#pragma once

#include <atomic>
#include <memory>
#include <string>
#include <thread>
#include <vector>

namespace grpc {
class Server;
class Service;
} // namespace grpc

namespace nativerpc {

class GrpcServer {
  public:
    // Defined out-of-line so callers don't need <grpcpp/grpcpp.h> just
    // to keep a GrpcServer member (unique_ptr<grpc::Server> requires the
    // full type at dtor-instantiation).
    GrpcServer();
    ~GrpcServer();
    GrpcServer(const GrpcServer&) = delete;
    GrpcServer& operator=(const GrpcServer&) = delete;

    // `services` are non-owning; the caller keeps them alive for the
    // lifetime of this GrpcServer.
    [[nodiscard]] bool Start(const std::string& uds_path,
                             const std::vector<grpc::Service*>& services);

    // Idempotent.
    void Shutdown();

    [[nodiscard]] bool running() const { return running_.load(); }

  private:
    std::unique_ptr<grpc::Server> server_;
    std::thread serve_thread_;
    std::atomic<bool> running_{false};
    std::string uds_path_;
};

} // namespace nativerpc
