#pragma once

#include "src/render/pl_compose.hpp"
#include "src/render/world.hpp"

namespace render {

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

[[nodiscard]] pl_compose::SourceSlot
build_source_slot(const FrameGeom& frame, const LayoutRect& rect, const SourceState* state);

} // namespace render
