// world — composer-side mirror of the daemon-pushed runtime state.
//
// `videonode-composer` is daemon-driven: argv is just `--drm-device
// --ctl-connect --composer-id`. The daemon pushes everything else
// (canvas dims, slot ↔ source bindings, layout, effects, per-source
// state) over the JSON-RPC control channel. World is the in-memory
// mirror of that state, mutex-protected so the RPC handler thread can
// mutate and the canvas-loop thread can take consistent snapshots.
//
// Concurrency model
// -----------------
//   Writers:  RPC handler callbacks (single thread, the one driving
//             ControlChannel::handle_events).
//   Readers:  the canvas-loop render thread, via World::snapshot().
//
//   Both lock the same internal mutex. `snapshot()` returns a value-copy
//   (cheap — small POD-ish struct plus a few short vectors) so the
//   render loop never holds the lock during compose/render/readback.
//
// "Ready" semantics
// -----------------
//   World starts un-ready. It flips ready=true once the daemon has
//   pushed at least one `set_canvas` AND at least one `set_source` with
//   a placement in `set_layout`. Until then, the canvas loop renders a
//   solid-black BGRA frame so downstream encoders keep flowing.
//
// Effect application
// ------------------
//   set_effects {source_id, effects[]} replaces the full effect list
//   for that source_id atomically. Today only effects[i].type ==
//   "perspective" is honored; the solved 3×3 homography is cached on
//   `World::SourceState::warp`. Unknown effect types are kept in the
//   list as a record (recognized=false) but don't contribute to the
//   warp — future composer builds will pick them up.
//
//   When source_state[source_id] is "placeholder", composer applies
//   identity (not the cached warp) so the source's NO-SIGNAL overlay
//   stays readable.

#pragma once

#include "src/rpc/composer_rpc.hpp"

#include <array>
#include <cstdint>
#include <map>
#include <mutex>
#include <optional>
#include <string>
#include <vector>

namespace render {

// SourceState collects everything World knows about one source, keyed by
// the daemon-issued source_id. The cached `warp` is the solved 3×3
// homography (row-major, GLSL mat3 layout) — identity if no perspective
// effect is set.
struct SourceState {
    std::string source_id;
    std::array<float, 9> warp = {1, 0, 0, 0, 1, 0, 0, 0, 1};
    bool has_perspective = false;              // true once set_effects with type=perspective lands
    std::string state = "live";                // "live" | "transitioning" | "placeholder"
    std::vector<composer_rpc::Effect> effects; // raw effect list (incl. unrecognized); audit/log
};

// SlotBinding ties a slot key ("a" / "b" / …) to a source_id and the
// SCM socket path the source is broadcasting on. ScmRightsSource is
// owned elsewhere (canvas_loop builds it from this binding); World
// only carries the bookkeeping.
struct SlotBinding {
    std::string slot; // "a", "b", …
    std::string source_id;
    std::string scm_path;
    uint32_t width = 0;
    uint32_t height = 0;
    uint32_t fps = 0;
};

// LayoutRect: where on the canvas to draw a given slot.
struct LayoutRect {
    std::string slot;
    int32_t x = 0;
    int32_t y = 0;
    int32_t w = 0;
    int32_t h = 0;
    int32_t rotation = 0;          // 0, 90, 180, 270 clockwise degrees
    int32_t aspect_ratio_mode = 0; // 0=stretch, 1=fit, 2=crop
    float crop_x = 0.0f;           // 0-1 normalized horizontal crop offset (0.5 = centered)
    float crop_y = 0.0f;           // 0-1 normalized vertical crop offset (0.5 = centered)
    float crop_scale = 1.0f;       // >= 1.0, source overfill factor
};

// Snapshot is what the render loop gets per frame. Cheap value-copy of
// the readable parts of World, taken under the lock.
struct Snapshot {
    bool ready = false;
    uint32_t canvas_w = 0;
    uint32_t canvas_h = 0;
    uint32_t canvas_fps = 0;
    uint32_t background_rgba = 0x000000FFU; // packed 0xRRGGBBAA, opaque black default
    std::vector<SlotBinding> slots;
    std::vector<LayoutRect> layout;
    std::map<std::string, SourceState> source_states; // key: source_id
};

class World {
  public:
    World() = default;
    World(const World&) = delete;
    World& operator=(const World&) = delete;

    // RPC apply methods. Each takes the already-parsed strong-typed
    // request from composer_rpc and updates the World atomically.
    // Returns false (with `err` populated) if the request was structurally
    // valid (parser accepted it) but semantically rejected by World —
    // e.g. perspective with bad snapshot dims, or set_layout referencing
    // a slot key that's never been bound.
    [[nodiscard]] bool apply_set_canvas(const composer_rpc::SetCanvasRequest& r,
                                        composer_rpc::ParseError& err);
    [[nodiscard]] bool apply_set_source(const composer_rpc::SetSourceRequest& r,
                                        composer_rpc::ParseError& err);
    [[nodiscard]] bool apply_clear_source(const composer_rpc::ClearSourceRequest& r,
                                          composer_rpc::ParseError& err);
    [[nodiscard]] bool apply_set_layout(const composer_rpc::SetLayoutRequest& r,
                                        composer_rpc::ParseError& err);
    [[nodiscard]] bool apply_set_effects(const composer_rpc::SetEffectsRequest& r,
                                         composer_rpc::ParseError& err);
    [[nodiscard]] bool apply_set_source_state(const composer_rpc::SetSourceStateRequest& r,
                                              composer_rpc::ParseError& err);

    // snapshot() returns a value-copy of the readable state. Cheap; the
    // render loop calls this per frame. Lock held only during the copy.
    Snapshot snapshot() const;

  private:
    mutable std::mutex mu_;
    uint32_t canvas_w_ = 0;
    uint32_t canvas_h_ = 0;
    uint32_t canvas_fps_ = 0;
    uint32_t background_rgba_ = 0x000000FFU;
    bool got_canvas_ = false;
    std::map<std::string, SlotBinding> slots_;         // key: slot name
    std::map<std::string, LayoutRect> layout_;         // key: slot name
    std::map<std::string, SourceState> source_states_; // key: source_id
};

} // namespace render
