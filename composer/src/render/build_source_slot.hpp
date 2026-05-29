// build_source_slot — pure mapping from a producer frame + its canvas
// placement into a pl_compose::SourceSlot.
//
// Split out of canvas_loop's per-frame render-batch builder so the
// geometry math (pitch/offset passthrough, fit/crop aspect-ratio,
// rotation, perspective warp) is unit-testable on the host with no GPU,
// EGL, or dma-buf — none of which the mapping itself needs.

#pragma once

#include "src/render/pl_compose.hpp" // pl_compose::SourceSlot
#include "src/render/world.hpp"      // render::LayoutRect, render::SourceState

namespace render {

// FrameGeom is the copyable subset of scm_rights_source::OwnedFrameView
// that slot-building reads. Carries borrowed fds (ints) plus plane
// pitches and byte offsets; owns nothing.
struct FrameGeom {
    int y_fd = -1;
    int uv_fd = -1;
    int width = 0;
    int height = 0;
    int y_pitch = 0;  // 0 → derive from width
    int uv_pitch = 0; // 0 → derive from width
    int y_offset = 0;
    int uv_offset = 0;
};

// build_source_slot maps one bound source frame and its layout rect into
// a SourceSlot. `state` (nullable) supplies the perspective warp; it is
// applied only when present, not in placeholder state, and has a solved
// homography. Pure: no syscalls, no GPU.
[[nodiscard]] pl_compose::SourceSlot
build_source_slot(const FrameGeom& frame, const LayoutRect& rect, const SourceState* state);

} // namespace render
