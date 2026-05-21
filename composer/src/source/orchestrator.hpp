// SourceOrchestrator — argv parsing + capture/broadcast lifecycle for the
// videonode-source sidecar. Pulled out of bin/videonode_source_main.cpp so
// the binary entry point stays signal-handler-only.
#pragma once

#include <atomic>
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
    // DRM render node used by the GBM allocator on Fedora / Mesa hosts.
    // Ignored when HAVE_RGA is on (rig uses dma_heap; no GBM needed).
    std::string alloc_drm_device = "/dev/dri/renderD128";
};

// Parse argv into Args. Returns false on unknown flag / missing value.
// `--help` / `--version` call exit() directly.
bool parse_args(int argc, char** argv, Args& a);

// Print usage to stdout. `d` supplies default values for the message.
void print_help(const Args& d);

// Run the capture + broadcast loop until `running` goes false. Returns
// process exit code (0 = ok, non-zero = startup failure).
int Run(const Args& a, std::atomic<bool>& running);

} // namespace source
