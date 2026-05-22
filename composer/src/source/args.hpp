// source::Args — videonode-source argv schema. Kept in its own header so
// broadcast.hpp can reference it without dragging in orchestrator.hpp,
// which would create a cycle (orchestrator.cpp pulls broadcast.hpp).
#pragma once

#include <string>

namespace source {

struct Args {
    std::string device = "/dev/video0";
    std::string in_format;
    int in_width = 0;
    int in_height = 0;
    int in_fps = 0;
    int buffers = 4;
    std::string out_socket = "/tmp/videonode-source.sock";
    int max_consumers = 16;
    int run_seconds = 0;
    int broadcast_fps = 60;
    int placeholder_w = 1920;
    int placeholder_h = 1080;
    // Control plane: if both set, sidecar dials the daemon and identifies
    // itself; otherwise control-plane is disabled (back-compat for
    // standalone runs from the smoke script / dev shell).
    std::string ctl_connect;
    std::string device_id;
    // gRPC control plane: when set, videonode-source binds a gRPC server
    // on this UDS and the daemon dials in (replaces ctl_connect after
    // the full cutover). Empty = no gRPC server.
    std::string grpc_listen;
    // DRM render node used by the GBM allocator on Fedora / Mesa hosts.
    // Ignored when HAVE_RGA is on (rig uses dma_heap; no GBM needed).
    std::string alloc_drm_device = "/dev/dri/renderD128";
};

} // namespace source
