// videonode-composer — daemon-driven GPU compositor.
//
// argv is intentionally minimal — `videonode-composer` is a passive
// render server. The daemon dials `--ctl-connect` (a Unix socket it's
// listening on), receives our `identify`, and pushes everything
// dynamic (canvas dims, slot ↔ source bindings, layout, effects,
// per-source state) as JSON-RPC requests. We snapshot that state every
// frame in canvas_loop, render BGRA to stdout. Pipe stdout to ffmpeg
// for transcoding.
//
//   videonode-composer
//       --drm-device /dev/dri/renderD130
//       --ctl-connect /tmp/videonode-control.sock
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
#include "src/rpc/composer_rpc.hpp"
#include "src/rpc/control_channel.hpp"
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
    std::string ctl_connect; // empty = control plane disabled (diagnostic mode)
    std::string composer_id;
    // gRPC control plane: when set, composer binds a gRPC server here.
    // Empty = no gRPC server (parallel to ctl_connect for the cutover).
    std::string grpc_listen;
    int run_seconds = 0; // 0 = until SIGINT / stdout EPIPE
    int target_fps = 30; // pre-ready tick rate; once ready, snapshot.canvas_fps wins
};

void print_help(const Args& d) {
    printf("videonode-composer — daemon-driven BGRA canvas writer.\n"
           "\n"
           "  --drm-device PATH      DRM render node (default %s)\n"
           "  --ctl-connect PATH     daemon UDS path for JSON-RPC control plane (required for live "
           "config)\n"
           "  --composer-id ID       stable identifier sent to daemon on identify (required if "
           "--ctl-connect set)\n"
           "  --seconds N            run length in seconds (default %d = until SIGINT / stdout "
           "closes)\n"
           "  --target-fps N         pre-ready (no canvas yet) tick rate (default %d); once daemon "
           "sends set_canvas\n"
           "                           the snapshot's canvas_fps takes over\n"
           "  --version              print version and exit\n"
           "\n"
           "Stdout: raw BGRA bytes at canvas_w*canvas_h*4 per frame. Pipe to\n"
           "ffmpeg with `-f rawvideo -pix_fmt bgra -s WxH -framerate N -i pipe:0 …`\n"
           "(W/H/N come from the daemon's set_canvas push — pick matching values\n"
           "in the downstream ffmpeg invocation).\n",
           d.drm_device.c_str(), d.run_seconds, d.target_fps);
}

// Build the control-channel command handler. Each daemon-issued method
// parses the params with composer_rpc, then applies to World.
control_channel::HandlerResponse dispatch_command(render::World& world,
                                                  const control_channel::IncomingRequest& req) {
    using composer_rpc::ParseError;

    auto mk_err = [&](const ParseError& e) {
        control_channel::HandlerResponse r;
        r.ok = false;
        r.error_code = e.code;
        r.error_message = e.message;
        return r;
    };
    auto mk_ok = []() {
        control_channel::HandlerResponse r;
        r.ok = true;
        r.result_json = "{}";
        return r;
    };

    if (req.method == "set_canvas") {
        composer_rpc::SetCanvasRequest p;
        ParseError e;
        if (!composer_rpc::parse_set_canvas(req.params_json, p, e))
            return mk_err(e);
        if (!world.apply_set_canvas(p, e))
            return mk_err(e);
        return mk_ok();
    }
    if (req.method == "set_source") {
        composer_rpc::SetSourceRequest p;
        ParseError e;
        if (!composer_rpc::parse_set_source(req.params_json, p, e))
            return mk_err(e);
        if (!world.apply_set_source(p, e))
            return mk_err(e);
        return mk_ok();
    }
    if (req.method == "clear_source") {
        composer_rpc::ClearSourceRequest p;
        ParseError e;
        if (!composer_rpc::parse_clear_source(req.params_json, p, e))
            return mk_err(e);
        if (!world.apply_clear_source(p, e))
            return mk_err(e);
        return mk_ok();
    }
    if (req.method == "set_layout") {
        composer_rpc::SetLayoutRequest p;
        ParseError e;
        if (!composer_rpc::parse_set_layout(req.params_json, p, e))
            return mk_err(e);
        if (!world.apply_set_layout(p, e))
            return mk_err(e);
        return mk_ok();
    }
    if (req.method == "set_effects") {
        composer_rpc::SetEffectsRequest p;
        ParseError e;
        if (!composer_rpc::parse_set_effects(req.params_json, p, e))
            return mk_err(e);
        if (!world.apply_set_effects(p, e))
            return mk_err(e);
        return mk_ok();
    }
    if (req.method == "set_source_state") {
        composer_rpc::SetSourceStateRequest p;
        ParseError e;
        if (!composer_rpc::parse_set_source_state(req.params_json, p, e))
            return mk_err(e);
        if (!world.apply_set_source_state(p, e))
            return mk_err(e);
        return mk_ok();
    }
    if (req.method == "shutdown") {
        // Trigger a clean exit. Render loop sees g_running flip below.
        g_running.store(false);
        return mk_ok();
    }
    // Unknown method — JSON-RPC standard "method not found".
    control_channel::HandlerResponse r;
    r.ok = false;
    r.error_code = -32601;
    r.error_message = "method not found: " + req.method;
    return r;
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
        else if (s == "--ctl-connect")
            i = eat_str(i, a.ctl_connect);
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

    // Control channel is optional. Two parallel paths live alongside
    // until the JSON-RPC code is deleted in a later commit:
    //   --grpc-listen  → run a gRPC server on the given UDS (new)
    //   --ctl-connect  → dial the daemon over JSON-RPC (legacy)
    // Either / both / neither may be set; standalone (neither) renders
    // black until SIGINT.
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
    }

    control_channel::ControlChannel ctl;
    control_channel::ControlChannel* ctl_ptr = nullptr;
    if (!a.ctl_connect.empty() && !a.composer_id.empty()) {
        ctl.init(a.ctl_connect, a.composer_id, vn::kVersion, "composer");
        ctl.set_command_handler([&](const control_channel::IncomingRequest& req) {
            return dispatch_command(world, req);
        });
        ctl_ptr = &ctl;
        fprintf(stderr, "ok: control channel target=%s id=%s\n", a.ctl_connect.c_str(),
                a.composer_id.c_str());
    } else if (a.grpc_listen.empty()) {
        fprintf(stderr, "WARN: control channel disabled (missing --grpc-listen / --ctl-connect) — "
                        "composer will render black until SIGINT\n");
    }

    int frames = render::RunCanvasLoop(ctx, world, ctl_ptr, a.target_fps, a.run_seconds, g_running);

    grpc_srv.Shutdown();

    fprintf(stderr, "PASS: %d frames composed\n", frames);
    return 0;
}
