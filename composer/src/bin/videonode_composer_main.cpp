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

#include "src/render/canvas_loop.hpp"
#include "src/render/composer_service.hpp"
#include "src/render/egl_ctx.hpp"
#include "src/render/world.hpp"
#include "src/rpc/grpc_server.hpp"
#include "version.hpp"

#include <atomic>
#include <csignal>
#include <cstdio>
#include <cstdlib>
#include <memory>
#include <string>
#include <vector>

namespace {

std::atomic<bool> g_running{true};
void on_signal(int) {
    g_running.store(false);
}

struct Args {
    std::string drm_device = "/dev/dri/renderD128";
    // gRPC control plane: when set together with --composer-id, the
    // composer binds a gRPC server here and the daemon dials in. Empty
    // = standalone (renders solid black until SIGINT).
    std::string grpc_listen;
    std::string composer_id;
    int run_seconds = 0; // 0 = until SIGINT / stdout EPIPE
    int target_fps = 30; // pre-ready tick rate; once ready, snapshot.canvas_fps wins
};

void print_help(const Args& d) {
    printf("videonode-composer — daemon-driven BGRA canvas writer.\n"
           "\n"
           "  --drm-device PATH      DRM render node (default %s)\n"
           "  --grpc-listen PATH     per-instance UDS the composer's gRPC server binds;\n"
           "                           the daemon dials in. Required for live config.\n"
           "  --composer-id ID       stable identifier advertised via Composer.Describe()\n"
           "                           (required when --grpc-listen is set)\n"
           "  --seconds N            run length in seconds (default %d = until SIGINT / stdout "
           "closes)\n"
           "  --target-fps N         pre-ready (no canvas yet) tick rate (default %d); once daemon\n"
           "                           sends SetCanvas the snapshot's canvas_fps takes over\n"
           "  --version              print version and exit\n"
           "\n"
           "Stdout: raw BGRA bytes at canvas_w*canvas_h*4 per frame. Pipe to\n"
           "ffmpeg with `-f rawvideo -pix_fmt bgra -s WxH -framerate N -i pipe:0 …`\n"
           "(W/H/N come from the daemon's SetCanvas push — pick matching values\n"
           "in the downstream ffmpeg invocation).\n",
           d.drm_device.c_str(), d.run_seconds, d.target_fps);
}

} // namespace

int main(int argc, char** argv) {
    Args a;
    Args d; // defaults for help text

    auto eat_int = [&](int i, int& dst) -> int {
        if (i + 1 < argc) {
            dst = std::atoi(argv[i + 1]);
            return i + 1;
        }
        return i;
    };
    auto eat_str = [&](int i, std::string& dst) -> int {
        if (i + 1 < argc) {
            dst = argv[i + 1];
            return i + 1;
        }
        return i;
    };

    for (int i = 1; i < argc; ++i) {
        std::string s = argv[i];
        if (s == "--drm-device")
            i = eat_str(i, a.drm_device);
        else if (s == "--composer-id")
            i = eat_str(i, a.composer_id);
        else if (s == "--grpc-listen")
            i = eat_str(i, a.grpc_listen);
        else if (s == "--seconds")
            i = eat_int(i, a.run_seconds);
        else if (s == "--target-fps")
            i = eat_int(i, a.target_fps);
        else if (s == "-h" || s == "--help") {
            print_help(d);
            return 0;
        } else if (s == "--version") {
            printf("videonode-composer %s\n", vn::kVersion);
            return 0;
        } else if (!s.empty() && s[0] == '-') {
            fprintf(stderr, "unknown flag: %s (use --help)\n", s.c_str());
            return 2;
        }
    }

    std::signal(SIGINT, on_signal);
    std::signal(SIGTERM, on_signal);
    std::signal(SIGPIPE, SIG_IGN); // composer dies via EPIPE in write_full_

    egl_ctx::EglCtx ctx;
    if (!ctx.init(a.drm_device))
        return 1;

    render::World world;

    // Control plane is optional. Without --grpc-listen + --composer-id
    // the composer renders solid black until SIGINT — useful only for
    // diagnostic smoke tests.
    nativerpc::GrpcServer grpc_srv;
    std::unique_ptr<nativerpc::ComposerService> grpc_svc;
    if (!a.grpc_listen.empty() && !a.composer_id.empty()) {
        nativerpc::ComposerContext gctx;
        gctx.world = &world;
        gctx.running = &g_running;
        gctx.composer_id = a.composer_id;
        gctx.version = vn::kVersion;
        grpc_svc = std::make_unique<nativerpc::ComposerService>(std::move(gctx));
        std::vector<grpc::Service*> services = {grpc_svc.get()};
        if (!grpc_srv.Start(a.grpc_listen, services)) {
            fprintf(stderr, "FATAL: gRPC server failed to start on %s\n", a.grpc_listen.c_str());
            return 1;
        }
        fprintf(stderr, "ok: grpc server listening on %s id=%s\n", a.grpc_listen.c_str(),
                a.composer_id.c_str());
    } else {
        fprintf(stderr, "WARN: control plane disabled (missing --grpc-listen / --composer-id) — "
                        "composer will render black until SIGINT\n");
    }

    int frames = render::RunCanvasLoop(ctx, world, a.target_fps, a.run_seconds, g_running);

    grpc_srv.Shutdown();

    fprintf(stderr, "PASS: %d frames composed\n", frames);
    return 0;
}
