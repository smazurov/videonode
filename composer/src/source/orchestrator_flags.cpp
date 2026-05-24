// videonode-source CLI flag definitions + Args materialization. Split
// out of orchestrator.cpp so the main capture/broadcast loop stays under
// the 500-line soft cap (composer/CLAUDE.md: "split rather than
// thicken"). Defaults match source::Args field defaults; help text is
// the counterpart to the old print_help() block — absl auto-formats it
// into --help output (which also lists --version automatically).

#include "src/source/orchestrator_flags.hpp"

#include <absl/flags/flag.h>

#include <string>

ABSL_FLAG(std::string, device, "",
          "V4L2 capture device path (/dev/videoN). Empty = no device; the "
          "source paints placeholders and waits for SetDevice via gRPC.");
ABSL_FLAG(std::string, in_format, "",
          "input pixel format: NV24/NV16/NV12/BGR3/YUYV/UYVY/MJPG (empty = auto)");
ABSL_FLAG(int, in_width, 0, "input frame width when --in_format is set");
ABSL_FLAG(int, in_height, 0, "input frame height when --in_format is set");
ABSL_FLAG(int, in_fps, 0, "requested capture rate");
ABSL_FLAG(int, buffers, 4, "V4L2 ring size");
ABSL_FLAG(std::string, out_socket, "/tmp/videonode-source.sock",
          "Unix socket to publish NV12 dma-bufs on");
ABSL_FLAG(int, max_consumers, 16, "soft cap on concurrent consumers");
ABSL_FLAG(int, seconds, 0, "stop after N seconds (0 = until SIGINT)");
ABSL_FLAG(int, placeholder_broadcast_fps, 60,
          "placeholder/keep-alive broadcast cadence (Hz). Only paces the loop "
          "when the source is not Live; Live frames go out at the V4L2 "
          "capture rate.");
ABSL_FLAG(int, placeholder_w, 1920, "placeholder canvas width");
ABSL_FLAG(int, placeholder_h, 1080, "placeholder canvas height");
ABSL_FLAG(std::string, grpc_listen, "",
          "per-instance UDS where the source's gRPC server binds "
          "(the daemon dials in). Omit for standalone");
ABSL_FLAG(std::string, device_id, "",
          "stable device ID advertised via Source.Describe() "
          "(required when --grpc_listen is set)");
ABSL_FLAG(std::string, alloc_drm_device, "/dev/dri/renderD128",
          "DRM render node for the GBM allocator (host builds only; ignored on HAVE_RGA)");

namespace source {

Args BuildArgsFromFlags() {
    Args a;
    a.device = absl::GetFlag(FLAGS_device);
    a.in_format = absl::GetFlag(FLAGS_in_format);
    a.in_width = absl::GetFlag(FLAGS_in_width);
    a.in_height = absl::GetFlag(FLAGS_in_height);
    a.in_fps = absl::GetFlag(FLAGS_in_fps);
    a.buffers = absl::GetFlag(FLAGS_buffers);
    a.out_socket = absl::GetFlag(FLAGS_out_socket);
    a.max_consumers = absl::GetFlag(FLAGS_max_consumers);
    a.run_seconds = absl::GetFlag(FLAGS_seconds);
    a.placeholder_broadcast_fps = absl::GetFlag(FLAGS_placeholder_broadcast_fps);
    a.placeholder_w = absl::GetFlag(FLAGS_placeholder_w);
    a.placeholder_h = absl::GetFlag(FLAGS_placeholder_h);
    a.grpc_listen = absl::GetFlag(FLAGS_grpc_listen);
    a.device_id = absl::GetFlag(FLAGS_device_id);
    a.alloc_drm_device = absl::GetFlag(FLAGS_alloc_drm_device);
    return a;
}

} // namespace source
