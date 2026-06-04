#pragma once

#include <cstddef>
#include <cstdint>
#include <span>

namespace placeholder_painter {

// Constant so tests can verify paint_tick() only touches these rows.
struct AnimRegion {
    int y_start; // top scanline (inclusive)
    int y_end;   // bottom scanline (exclusive)
};

AnimRegion derive_anim_region(int w, int h);

[[nodiscard]] bool paint_base(std::span<uint8_t> nv12, int w, int h, const char* device_path);

[[nodiscard]] bool paint_tick(std::span<uint8_t> nv12, int w, int h, uint64_t tick_idx,
                              uint64_t wallclock_ms, const char* status);

} // namespace placeholder_painter
