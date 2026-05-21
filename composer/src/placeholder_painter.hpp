// placeholder_painter — paint a calm "NO SIGNAL DETECTED" NV12 frame.
//
// Used by videonode-source when V4L2 stops yielding frames. The downstream
// composer + encoder + RTSP path is identical to real frames; only the
// pixels are synthetic.
//
// Two operations split for cost:
//   paint_base() — full repaint (background fill, static title text).
//                  Call once per buffer or after dimensions change.
//   paint_tick() — update animated region only (spinner + live timestamp).
//                  Call once per broadcast tick. Cheap.
//
// Both write NV12 in place: luma plane [0 .. w*h), interleaved CbCr plane
// [w*h .. w*h*3/2). Chroma is sub-sampled half-W half-H (4:2:0).

#pragma once

#include <cstddef>
#include <cstdint>
#include <span>

namespace placeholder_painter {

// Geometry of the per-tick animated region. Constant so tests can verify
// paint_tick() only touches these rows. The strip is wide enough for the
// timestamp text + spinner; sits centered horizontally below the title.
struct AnimRegion {
    int y_start; // top scanline (inclusive)
    int y_end;   // bottom scanline (exclusive)
};

// derive_anim_region returns the y-range paint_tick() writes into for a
// canvas of given dimensions. The region is centered vertically below the
// "NO SIGNAL DETECTED" title.
AnimRegion derive_anim_region(int w, int h);

// paint_base fills the luma + chroma planes with the background color,
// renders the "NO SIGNAL DETECTED" headline, and prints the device path
// as a subtitle (uppercased — the font is uppercase-only). Idempotent.
//
// device_path may be empty to omit the subtitle. Longer than the canvas
// width gets truncated.
//
// Requires:
//   nv12_size >= w * h * 3 / 2
//   w, h even and >= 256 (we don't bother scaling text below that)
void paint_base(uint8_t* nv12, int w, int h, const char* device_path);

// Span overload of paint_base. Returns false (no-op) if the span is too
// small to hold an NV12 frame of the given dimensions.
bool paint_base(std::span<uint8_t> nv12, int w, int h, const char* device_path);

// paint_tick repaints the animated region with a fresh spinner frame, the
// current wall-clock timestamp + frame counter, and a status line
// (e.g. "NO CABLE DETECTED" or "WAITING FOR SIGNAL"). tick_idx drives
// spinner rotation; wallclock_ms is the timestamp value.
void paint_tick(uint8_t* nv12, int w, int h, uint64_t tick_idx, uint64_t wallclock_ms,
                const char* status);

// Span overload of paint_tick. Returns false (no-op) if the span is too
// small to hold an NV12 frame of the given dimensions.
bool paint_tick(std::span<uint8_t> nv12, int w, int h, uint64_t tick_idx, uint64_t wallclock_ms,
                const char* status);

} // namespace placeholder_painter
