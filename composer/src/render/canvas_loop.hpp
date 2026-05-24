// canvas_loop — daemon-driven composer render loop.
//
// videonode-composer is a passive render server: its argv carries only
// `--drm-device --grpc-listen --composer-id`. Everything dynamic (canvas
// dims, source bindings, layout, per-source effects, per-source state)
// arrives over gRPC and lands in `World`. `RunCanvasLoop` snapshots
// World per frame, materializes the bound sources (dialling/tearing
// down ScmRightsSource instances to match the World's current slot
// map), runs `gl_compose`, gbm_bo_maps the canvas, and writes BGRA
// bytes to stdout.
//
// Lifecycles
// ----------
//   gl_compose is created lazily on the first "ready" snapshot (so we
//   wait until the daemon has told us the canvas dims). MVP: canvas
//   dimension changes mid-stream log a one-line WARN and are ignored —
//   gl_compose is constructed once with the first ready dims and stays.
//
//   ScmRightsSource instances are owned by canvas_loop, one per slot.
//   When a new slot/scm_path appears in the snapshot, we dial + start.
//   When a slot disappears or its scm_path changes, we stop + drop.
//   No per-frame work on already-running sources beyond `latest_frame()`.
//
// Until World is ready (canvas + at least one bound+placed slot),
// RunCanvasLoop writes a solid-black BGRA frame each tick so any
// downstream ffmpeg pipe keeps flowing.

#pragma once

#include <atomic>
#include <cstdint>
#include <string>

namespace egl_ctx {
class EglCtx;
}
namespace render {
class World;
}
namespace nativerpc {
class ComposerService;
}

namespace render {

// Lock-free counters the canvas loop writes once per frame and the gRPC
// `Composer.GetStats` handler reads from another thread. fps_observed is
// stored as centi-fps (fps × 100) so it can fit in a plain atomic and
// stays consistent with the rest of the snapshot.
struct RenderStats {
    std::atomic<uint64_t> frames_rendered{0};
    std::atomic<uint32_t> fps_observed_centi{0}; // fps * 100, recomputed every ~1s
    std::atomic<uint32_t> canvas_w{0};
    std::atomic<uint32_t> canvas_h{0};
    std::atomic<uint32_t> canvas_fps{0};
    std::atomic<int32_t> consumer_count{0}; // SCM mode only; 0 in stdout mode
};

// Render at the target frame rate until `running` goes false, the
// composer's stdout closes (EPIPE in stdout mode) or all SCM consumers
// hang up after running once (SCM mode is fanout-tolerant), or
// `run_seconds` (if non-zero) elapses.
//
// `target_fps` is the loop's tick rate; `world.snapshot().canvas_fps`
// is preferred once ready, but until then we tick at this rate.
//
// `scm_out_path` selects the output mode:
//   - empty       → legacy: BGRA bytes go to stdout (pipe to ffmpeg)
//   - non-empty   → SCM_RIGHTS: listen on the path, broadcast canvas
//                   dma-buf fd + dmabuf_header::Header to all consumers
//                   (vn-sink → ffmpeg, etc.). Decouples composer lifetime
//                   from any one consumer; encoder restart no longer kills
//                   the composer via EPIPE.
//
// `stats` (optional) is updated each frame and on each consumer-prune
// tick. Pass nullptr when no observer is wired up (smoke tests, no-gRPC
// diagnostic runs).
//
// `composer_svc` (optional) receives a FrameRef pointing at the latest
// canvas dma-buf after each SCM broadcast. The service holds the ref so
// its Snapshot RPC can mmap+pack on demand instead of paying per frame.
// Pass nullptr when no gRPC server is wired up.
//
// Returns the number of frames rendered (placeholder + real).
int RunCanvasLoop(egl_ctx::EglCtx& ctx, World& world, int target_fps, int run_seconds,
                  std::atomic<bool>& running, const std::string& scm_out_path = "",
                  RenderStats* stats = nullptr, nativerpc::ComposerService* composer_svc = nullptr);

} // namespace render
