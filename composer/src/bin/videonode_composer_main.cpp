// videonode-composer — daemon-driven GPU compositor.
//
// A passive render server: the daemon binds via --grpc-listen, calls
// Composer.Describe() for identity, then pushes all dynamic state (canvas
// dims, bindings, layout, effects, per-source state) as unary gRPC calls.
// Until the daemon has pushed a ready canvas, it renders solid black.

#include "src/common/flags_compat.hpp"
#include "src/common/log_levels.hpp"
#include "src/common/raise_fd_limit.hpp"
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
#include <span>
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

// Distinct code for a permanent GPU/render-node-absent startup failure (78 ==
// EX_UNAVAILABLE). Operator signal / future hook; the daemon doesn't branch yet.
constexpr int kExitGpuUnavailable = 78;

[[nodiscard]] static bool check_gpu_available(const std::string& drm_device) {
    auto probe = [](const char* n) {
        egl_ctx::EglCtx c;
        return c.init(n);
    };
    bool gpu_ok = !drm_device.empty() && probe(drm_device.c_str());
    if (!gpu_ok) {
        for (const char* cand : render::kDrmRenderCandidates) {
            if (drm_device == cand)
                continue;
            if (probe(cand)) {
                gpu_ok = true;
                break;
            }
        }
    }
    return gpu_ok;
}

static void setup_grpc_service(const std::string& grpc_listen, const std::string& composer_id,
                               render::World& world, render::RenderStats& render_stats,
                               nativerpc::GrpcServer& grpc_srv,
                               std::unique_ptr<nativerpc::ComposerService>& grpc_svc) {
    if (!grpc_listen.empty() && !composer_id.empty()) {
        nativerpc::ComposerContext gctx;
        gctx.world = &world;
        gctx.running = &g_running;
        gctx.stats = &render_stats;
        gctx.composer_id = composer_id;
        gctx.version = vn::kVersion;
        grpc_svc = std::make_unique<nativerpc::ComposerService>(std::move(gctx));
        std::vector<grpc::Service*> services = {grpc_svc.get()};
        if (!grpc_srv.Start(grpc_listen, services)) {
            vn::log::fatal("videonode-composer: gRPC server failed to start on %s",
                           grpc_listen.c_str());
            std::exit(1);
        }
        vn::log::info("videonode-composer: grpc server listening on %s id=%s", grpc_listen.c_str(),
                      composer_id.c_str());
    } else {
        vn::log::warn("videonode-composer: control plane disabled "
                      "(missing --grpc-listen / --composer-id) — "
                      "composer will render black until SIGINT");
    }
}

} // namespace

int main(int argc, char** argv) {
    vn::raise_fd_limit();

    const std::span<char*> args(argv, static_cast<size_t>(argc));
    for (size_t i = 1; i < args.size(); ++i) {
        if (std::strcmp(args[i], "--version") == 0) {
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

    if (!check_gpu_available(drm_device)) {
        vn::log::fatal("videonode-composer: no usable GPU render node; "
                       "exiting non-retryable");
        return kExitGpuUnavailable;
    }

    render::World world;
    render::RenderStats render_stats;

    nativerpc::GrpcServer grpc_srv;
    std::unique_ptr<nativerpc::ComposerService> grpc_svc;
    setup_grpc_service(grpc_listen, composer_id, world, render_stats, grpc_srv, grpc_svc);

    // Seed pre-ready canvas dims so the first frame matches the encoder's
    // fixed `-s WxH` input; the daemon's SetCanvas can override later.
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

    int rc = render::RunCanvasLoop({
        .world = world,
        .target_fps = target_fps,
        .run_seconds = run_seconds,
        .running = g_running,
        .scm_out_path = scm_out,
        .stats = &render_stats,
        .composer_svc = grpc_svc.get(),
    });

    grpc_srv.Shutdown();

    if (rc == render::kCanvasRuntimeError) {
        vn::log::error("videonode-composer: canvas loop failed; exiting");
        return EXIT_FAILURE;
    }

    vn::log::info("videonode-composer: %d frames composed", rc);
    return 0;
}
