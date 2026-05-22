// canvas_loop — daemon-driven composer render loop.
//
// videonode-composer is a passive render server: its argv carries only
// `--drm-device --ctl-connect --composer-id`. Everything dynamic (canvas
// dims, source bindings, layout, per-source effects, per-source state)
// arrives over JSON-RPC on the control channel and lands in `World`.
// `RunCanvasLoop` snapshots World per frame, materializes the bound
// sources (dialling/tearing down ScmRightsSource instances to match the
// World's current slot map), runs `gl_compose`, gbm_bo_maps the canvas,
// and writes BGRA bytes to stdout.
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

namespace control_channel {
class ControlChannel;
}
namespace egl_ctx {
class EglCtx;
}
namespace render {
class World;
}

namespace render {

// Render at the target frame rate until `running` goes false, the
// composer's stdout closes (EPIPE), or `run_seconds` (if non-zero)
// elapses. `ctl` is optional (nullable) — when null, composer renders
// black forever (useful only for diagnostics).
//
// `target_fps` is the loop's tick rate; `world.snapshot().canvas_fps`
// is preferred once ready, but until then we tick at this rate.
//
// Returns the number of frames rendered (placeholder + real).
int RunCanvasLoop(egl_ctx::EglCtx& ctx,
                  World& world,
                  control_channel::ControlChannel* ctl,
                  int target_fps,
                  int run_seconds,
                  std::atomic<bool>& running);

} // namespace render
