#include "src/rpc/grpc_server.hpp"

#include <grpcpp/grpcpp.h>

#include <cerrno>
#include <cstdio>
#include <cstring>
#include <unistd.h>

namespace nativerpc {

GrpcServer::GrpcServer() = default;

GrpcServer::~GrpcServer() {
    Shutdown();
}

bool GrpcServer::Start(const std::string& uds_path,
                       const std::vector<grpc::Service*>& services) {
    if (uds_path.empty()) {
        fprintf(stderr, "grpc_server: empty uds path\n");
        return false;
    }
    // Best-effort unlink of a stale socket. If a real server is listening
    // we won't be able to bind anyway; this just avoids the EADDRINUSE
    // crash after an unclean shutdown.
    if (::access(uds_path.c_str(), F_OK) == 0) {
        if (::unlink(uds_path.c_str()) != 0) {
            fprintf(stderr, "grpc_server: unlink(%s) failed: %s\n", uds_path.c_str(),
                    std::strerror(errno));
            // continue anyway — bind will tell us if it's a real problem
        }
    }

    grpc::ServerBuilder builder;
    builder.AddListeningPort("unix:" + uds_path, grpc::InsecureServerCredentials());
    for (auto* svc : services) {
        builder.RegisterService(svc);
    }
    server_ = builder.BuildAndStart();
    if (!server_) {
        fprintf(stderr, "grpc_server: BuildAndStart failed on %s\n", uds_path.c_str());
        return false;
    }
    uds_path_ = uds_path;
    running_.store(true);
    serve_thread_ = std::thread([this] {
        // Wait() blocks until Shutdown() is called.
        server_->Wait();
        running_.store(false);
    });
    fprintf(stderr, "grpc_server: listening on unix:%s\n", uds_path.c_str());
    return true;
}

void GrpcServer::Shutdown() {
    if (server_) {
        server_->Shutdown();
    }
    if (serve_thread_.joinable()) {
        serve_thread_.join();
    }
    server_.reset();
    if (!uds_path_.empty()) {
        // Best effort — don't error if it's already gone.
        ::unlink(uds_path_.c_str());
        uds_path_.clear();
    }
    running_.store(false);
}

} // namespace nativerpc
