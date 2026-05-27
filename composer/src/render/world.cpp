#include "src/render/world.hpp"

#include "src/render/homography.hpp"

namespace render {

namespace {

constexpr int kInvalidParams = -32602;
constexpr int kSemanticError = -32000;

// Input bounds, matched to the deleted composer_rpc.cpp parsers so the
// gRPC handlers reject the same garbage the JSON-RPC parsers did. The
// upper bounds are intentionally generous — they catch obvious abuse
// (zero dims, gigabyte textures, runaway fps) without constraining real
// content (4K @ 240 fps fits comfortably).
constexpr uint32_t kMaxDim = 16384;
constexpr uint32_t kMaxFps = 240;

bool fail(composer_rpc::ParseError& err, int code, const char* msg) {
    err.code = code;
    err.message = msg;
    return false;
}

// Touch a source_state entry, creating it with default warp/state if
// unseen. Called from set_effects and set_source_state — the daemon may
// announce one before the other depending on event ordering.
SourceState& touch_source_state(std::map<std::string, SourceState>& m,
                                const std::string& source_id) {
    auto [it, inserted] = m.try_emplace(source_id);
    if (inserted)
        it->second.source_id = source_id;
    return it->second;
}

// Recompute the cached warp from the source's current effect list.
// Today only `perspective` contributes; future effect kinds (crop, bbox)
// will plug in here.
bool recompute_warp(SourceState& ss, composer_rpc::ParseError& err) {
    // Default: identity warp, no perspective.
    ss.warp = {1, 0, 0, 0, 1, 0, 0, 0, 1};
    ss.has_perspective = false;
    for (const auto& e : ss.effects) {
        if (!e.recognized)
            continue;
        if (e.type == "perspective" && e.perspective.has_value()) {
            const auto& p = *e.perspective;
            float h[9] = {};
            auto status = homography::corners_to_warp(p.corners, p.snapshot_w, p.snapshot_h, h);
            if (status == homography::Status::BadSnapshotDims)
                return fail(err, kInvalidParams, "perspective: snapshot dims invalid");
            if (status == homography::Status::Degenerate)
                return fail(err, kSemanticError, "perspective: degenerate corner quadrilateral");
            ss.warp = {h[0], h[1], h[2], h[3], h[4], h[5], h[6], h[7], h[8]};
            ss.has_perspective = true;
            // First recognized perspective wins; ignore later ones in the
            // same effect list. Composer applies one warp per source — if
            // the daemon wants two stacked transforms, that's a future
            // change to the apply pipeline.
            break;
        }
    }
    return true;
}

} // namespace

bool World::apply_set_canvas(const composer_rpc::SetCanvasRequest& r,
                             composer_rpc::ParseError& err) {
    if (r.w == 0 || r.h == 0)
        return fail(err, kInvalidParams, "set_canvas: w/h must be > 0");
    if (r.w > kMaxDim || r.h > kMaxDim)
        return fail(err, kInvalidParams, "set_canvas: w/h exceed 16384");
    if (r.fps == 0 || r.fps > kMaxFps)
        return fail(err, kInvalidParams, "set_canvas: fps must be in 1..240");
    std::lock_guard<std::mutex> g(mu_);
    canvas_w_ = r.w;
    canvas_h_ = r.h;
    canvas_fps_ = r.fps;
    got_canvas_ = true;
    return true;
}

bool World::apply_set_source(const composer_rpc::SetSourceRequest& r,
                             composer_rpc::ParseError& err) {
    if (r.slot.empty())
        return fail(err, kInvalidParams, "set_source: slot must be non-empty");
    if (r.source_id.empty())
        return fail(err, kInvalidParams, "set_source: source_id must be non-empty");
    if (r.scm_path.empty())
        return fail(err, kInvalidParams, "set_source: scm_path must be non-empty");
    if (r.width == 0 || r.height == 0)
        return fail(err, kInvalidParams, "set_source: width/height must be > 0");
    if (r.width > kMaxDim || r.height > kMaxDim)
        return fail(err, kInvalidParams, "set_source: width/height exceed 16384");
    if (r.fps > kMaxFps)
        return fail(err, kInvalidParams, "set_source: fps exceeds 240");
    std::lock_guard<std::mutex> g(mu_);
    SlotBinding b{};
    b.slot = r.slot;
    b.source_id = r.source_id;
    b.scm_path = r.scm_path;
    b.width = r.width;
    b.height = r.height;
    b.fps = r.fps;
    slots_[r.slot] = std::move(b);
    // Touch the source-state so it exists when set_effects / set_source_state
    // arrive (or has already arrived in the other order).
    (void)touch_source_state(source_states_, r.source_id);
    return true;
}

bool World::apply_clear_source(const composer_rpc::ClearSourceRequest& r,
                               composer_rpc::ParseError& err) {
    if (r.slot.empty())
        return fail(err, kInvalidParams, "clear_source: slot must be non-empty");
    std::lock_guard<std::mutex> g(mu_);
    auto it = slots_.find(r.slot);
    if (it == slots_.end())
        return fail(err, kSemanticError, "clear_source: slot not bound");
    slots_.erase(it);
    // Layout entry for the slot, if any, is left in place — the daemon
    // typically clears it via set_layout. Leaving it is harmless because
    // the render loop only iterates slots that exist in `slots_`.
    return true;
}

bool World::apply_set_layout(const composer_rpc::SetLayoutRequest& r,
                             composer_rpc::ParseError& err) {
    // Validate the whole list before touching layout_ so a partial
    // apply doesn't leave the composer in a half-updated state.
    for (const auto& ls : r.slots) {
        if (ls.slot.empty())
            return fail(err, kInvalidParams, "set_layout: empty slot name");
        if (ls.w <= 0 || ls.h <= 0)
            return fail(err, kInvalidParams, "set_layout: w/h must be > 0");
        if (ls.w > int32_t(kMaxDim) || ls.h > int32_t(kMaxDim))
            return fail(err, kInvalidParams, "set_layout: w/h exceed 16384");
        if (ls.rotation != 0 && ls.rotation != 90 && ls.rotation != 180 && ls.rotation != 270)
            return fail(err, kInvalidParams, "set_layout: rotation must be 0, 90, 180, or 270");
        if (ls.aspect_ratio_mode < 0 || ls.aspect_ratio_mode > 2)
            return fail(err, kInvalidParams, "set_layout: aspect_ratio_mode must be 0, 1, or 2");
    }
    std::lock_guard<std::mutex> g(mu_);
    layout_.clear();
    for (const auto& ls : r.slots) {
        LayoutRect rect;
        rect.slot = ls.slot;
        rect.x = ls.x;
        rect.y = ls.y;
        rect.w = ls.w;
        rect.h = ls.h;
        rect.rotation = ls.rotation;
        rect.aspect_ratio_mode = ls.aspect_ratio_mode;
        rect.crop_x = ls.crop_x;
        rect.crop_y = ls.crop_y;
        rect.crop_scale = ls.crop_scale;
        layout_[ls.slot] = std::move(rect);
    }
    return true;
}

bool World::apply_set_effects(const composer_rpc::SetEffectsRequest& r,
                              composer_rpc::ParseError& err) {
    if (r.source_id.empty())
        return fail(err, kInvalidParams, "set_effects: source_id must be non-empty");
    std::lock_guard<std::mutex> g(mu_);
    auto& ss = touch_source_state(source_states_, r.source_id);
    ss.effects = r.effects;
    return recompute_warp(ss, err);
}

bool World::apply_set_source_state(const composer_rpc::SetSourceStateRequest& r,
                                   composer_rpc::ParseError& err) {
    if (r.source_id.empty())
        return fail(err, kInvalidParams, "set_source_state: source_id must be non-empty");
    if (r.state != "live" && r.state != "transitioning" && r.state != "placeholder")
        return fail(err, kInvalidParams,
                    "set_source_state: state must be live|transitioning|placeholder");
    std::lock_guard<std::mutex> g(mu_);
    auto& ss = touch_source_state(source_states_, r.source_id);
    ss.state = r.state;
    return true;
}

Snapshot World::snapshot() const {
    std::lock_guard<std::mutex> g(mu_);
    Snapshot s;
    s.canvas_w = canvas_w_;
    s.canvas_h = canvas_h_;
    s.canvas_fps = canvas_fps_;
    // ready := got at least one canvas AND at least one slot binding AND
    // a layout entry for at least one bound slot. Otherwise the render
    // loop falls back to a black canvas (no source to compose yet).
    bool any_bound_and_placed = false;
    for (const auto& [slot, _] : slots_) {
        if (layout_.find(slot) != layout_.end()) {
            any_bound_and_placed = true;
            break;
        }
    }
    s.ready = got_canvas_ && any_bound_and_placed;
    s.slots.reserve(slots_.size());
    for (const auto& [_, b] : slots_)
        s.slots.push_back(b);
    s.layout.reserve(layout_.size());
    for (const auto& [_, r] : layout_)
        s.layout.push_back(r);
    s.source_states = source_states_;
    return s;
}

} // namespace render
