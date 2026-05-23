// videonode-composer — daemon-driven GPU compositor.
//
// argv is intentionally minimal: videonode-composer is a passive render
// server. The daemon dials `--grpc-listen` (a Unix socket the composer
// binds), calls Composer.Describe() to capture identity, and pushes
// everything dynamic (canvas dims, slot ↔ source bindings, layout,
// effects, per-source state) as unary gRPC calls. Canvas_loop snapshots
// that state every frame and renders BGRA to stdout. Pipe stdout to
// ffmpeg for transcoding.
//
//   videonode-composer
//       --drm-device /dev/dri/renderD130
//       --grpc-listen /tmp/videonode-native/composer-canvas-composer.sock
//       --composer-id <stream-id>-composer
//       --seconds 0
//
// Until the daemon has pushed at least one canvas + one bound+placed
// slot, we render a solid-black BGRA frame at a default 1280×720@30 so
// the downstream pipe stays alive.

#include "src/common/flags_compat.hpp"
#include "src/common/log_levels.hpp"
#include "src/common/signal.hpp"
#include "src/render/canvas_loop.hpp"
#include "src/render/composer_service.hpp"
#include "src/render/egl_ctx.hpp"
#include "src/render/world.hpp"
#include "src/rpc/composer_rpc.hpp"
#include "src/rpc/grpc_server.hpp"
#include "version.hpp"

#include <absl/flags/flag.h>
#include <absl/flags/parse.h>
#include <absl/flags/usage.h>

#include <atomic>
#include <cstdio>
#include <cstdlib>
#include <cstring>
#include <memory>
#include <string>
#include <vector>

ABSL_FLAG(std::string, drm_device, "/dev/dri/renderD128", "DRM render node");
ABSL_FLAG(std::string, grpc_listen, "",
          "per-instance UDS the composer's gRPC server binds; the daemon dials in. "
          "Required for live config");
ABSL_FLAG(std::string, composer_id, "",
          "stable identifier advertised via Composer.Describe() "
          "(required when --grpc_listen is set)");
ABSL_FLAG(std::string, scm_out, "",
          "listen on PATH and broadcast canvas dma-buf fd + dmabuf_header to all SCM "
          "consumers. Mutually exclusive with stdout-mode in practice — when set, "
          "stdout is no longer written");
ABSL_FLAG(int, seconds, 0, "run length in seconds (0 = until SIGINT / stdout closes)");
ABSL_FLAG(int, target_fps, 30,
          "pre-ready (no canvas yet) tick rate; once daemon sends SetCanvas the "
          "snapshot's canvas_fps takes over");
ABSL_FLAG(int, canvas_w, 0,
          "seed canvas width (default 1280); daemon's SetCanvas can override. Set when "
          "downstream ffmpeg consumes at fixed dims (-s WxH) to avoid first-frame size "
          "mismatches");
ABSL_FLAG(int, canvas_h, 0, "seed canvas height (default 720); see --canvas_w");

namespace {

std::atomic<bool> g_running{true};

} // namespace

int main(int argc, char** argv) {
    // Same hand-rolled --version dance as the other binaries: absl doesn't
    // own --version, and supervisors grep for the legacy spelling.
    for (int i = 1; i < argc; ++i) {
        if (std::strcmp(argv[i], "--version") == 0) {
            std::printf("videonode-composer %s\n", vn::kVersion);
            return 0;
        }
    }

    absl::SetProgramUsageMessage(
        "videonode-composer — daemon-driven canvas writer (stdout or SCM_RIGHTS).\n"
        "\n"
        "Output modes (pick exactly one path):\n"
        "  stdout (default): raw BGRA bytes at canvas_w*canvas_h*4 per frame.\n"
        "    Pipe to `ffmpeg -f rawvideo -pix_fmt bgra -s WxH -framerate N -i pipe:0 …`.\n"
        "    Composer dies via EPIPE if ffmpeg exits.\n"
        "  --scm_out PATH: SCM_RIGHTS broadcast of canvas dma-buf fd. Consumers dial\n"
        "    the socket (vn-sink, snapshot, etc.). Composer stays up across consumer\n"
        "    restarts.");
    vn::flags::configure_help_filter();
    vn::flags::normalize_argv(argc, argv);
    absl::ParseCommandLine(argc, argv);

    const std::string drm_device = absl::GetFlag(FLAGS_drm_device);
    const std::string grpc_listen = absl::GetFlag(FLAGS_grpc_listen);
    const std::string composer_id = absl::GetFlag(FLAGS_composer_id);
    const std::string scm_out = absl::GetFlag(FLAGS_scm_out);
    const int run_seconds = absl::GetFlag(FLAGS_seconds);
    const int target_fps = absl::GetFlag(FLAGS_target_fps);
    const int canvas_w = absl::GetFlag(FLAGS_canvas_w);
    const int canvas_h = absl::GetFlag(FLAGS_canvas_h);

    vn::signal::install_shutdown(g_running);

    egl_ctx::EglCtx ctx;
    if (!ctx.init(drm_device))
        return 1;

    render::World world;

    // Control plane is optional. Without --grpc-listen + --composer-id
    // the composer renders solid black until SIGINT — useful only for
    // diagnostic smoke tests.
    nativerpc::GrpcServer grpc_srv;
    std::unique_ptr<nativerpc::ComposerService> grpc_svc;
    if (!grpc_listen.empty() && !composer_id.empty()) {
        nativerpc::ComposerContext gctx;
        gctx.world = &world;
        gctx.running = &g_running;
        gctx.composer_id = composer_id;
        gctx.version = vn::kVersion;
        grpc_svc = std::make_unique<nativerpc::ComposerService>(std::move(gctx));
        std::vector<grpc::Service*> services = {grpc_svc.get()};
        if (!grpc_srv.Start(grpc_listen, services)) {
            vn::log::fatal("videonode-composer: gRPC server failed to start on %s",
                           grpc_listen.c_str());
            return 1;
        }
        vn::log::info("videonode-composer: grpc server listening on %s id=%s", grpc_listen.c_str(),
                      composer_id.c_str());
    } else {
        vn::log::warn("videonode-composer: control plane disabled "
                      "(missing --grpc-listen / --composer-id) — "
                      "composer will render black until SIGINT");
    }

    // Seed the World with the requested pre-ready canvas dims so the
    // very first frame is the correct size for the downstream encoder.
    // SetCanvas RPC from the daemon will later refresh these if it
    // disagrees. Required for inline-composer mode where the encoder's
    // ffmpeg input is invoked with `-s WxH` matching these dims.
    if (canvas_w > 0 && canvas_h > 0) {
        composer_rpc::SetCanvasRequest seed;
        seed.w = uint32_t(canvas_w);
        seed.h = uint32_t(canvas_h);
        seed.fps = uint32_t(target_fps);
        composer_rpc::ParseError seed_err;
        if (!world.apply_set_canvas(seed, seed_err)) {
            vn::log::warn("videonode-composer: seed canvas %dx%d rejected: %s", canvas_w, canvas_h,
                          seed_err.message.c_str());
        }
    }

    int frames = render::RunCanvasLoop(ctx, world, target_fps, run_seconds, g_running, scm_out);

    grpc_srv.Shutdown();

    vn::log::info("videonode-composer: %d frames composed", frames);
    return 0;
}
