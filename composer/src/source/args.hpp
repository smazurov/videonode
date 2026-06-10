// source::Args — videonode-source argv schema. Kept in its own header so
// broadcast.hpp can reference it without dragging in orchestrator.hpp,
// which would create a cycle (orchestrator.cpp pulls broadcast.hpp).
#pragma once

#include <string>

namespace source {

struct Args {
    std::string device = "/dev/video0";
    // Shell command whose stdout emits yuv4mpegpipe (y4m) frames; mutually
    // exclusive with device. Geometry/fps come from the y4m stream header.
    std::string pipe_cmd;
    std::string in_format;
    int in_width = 0;
    int in_height = 0;
    int in_fps = 0;
    int buffers = 4;
    std::string out_socket = "/tmp/videonode-source.sock";
    int max_consumers = 16;
    int run_seconds = 0;
    int placeholder_broadcast_fps = 60;
    int placeholder_w = 1920;
    int placeholder_h = 1080;
    // gRPC control plane: when set, videonode-source binds a gRPC server
    // on this UDS and the daemon dials in. Empty = no control plane
    // (standalone mode, used by the R smoke scenarios).
    std::string grpc_listen;
    // device-id is the stable identifier the source advertises via
    // Source.Describe() once the daemon dials in. Required when
    // --grpc-listen is set.
    std::string device_id;
    // DRM render node used by the GBM allocator on Fedora / Mesa hosts.
    // Ignored when HAVE_RGA is on (rig uses dma_heap; no GBM needed).
    std::string alloc_drm_device = "/dev/dri/renderD128";
};

} // namespace source
