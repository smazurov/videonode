// CSC backends are selected at build time:
//   - HAVE_RGA + HAVE_PLACEBO_CSC → RGA (fixed-function) first, with a
//     libplacebo GPU fallback for formats RGA can't convert (NV24 / YUV 4:4:4
//     on RK3588). The chosen path is latched per source-format.
//   - HAVE_RGA only               → Rockchip RGA (librga).
//   - HAVE_PLACEBO_CSC only       → libplacebo GPU (Mesa boxes).
//
// If no backend is compiled in, convert() logs once and returns false; the
// caller drops the frame.
//
// Output is always NV12 / MPEG-2 siting; luma matrix/range per call via
// ConvertParams::color_space (Default = BT.601 limited, Bt709Limited = BT.709).

#pragma once

#include <cstdint>

namespace csc {

// Bgra is the composer canvas (ARGB8888 byte order).
enum class PixelFormat {
    Nv12,
    Nv16,
    Nv24,
    Bgr3,
    Bgra,
    Yuyv,
    Uyvy,
};

enum class ColorSpace {
    Default,
    Bt709Limited,
};

// uv_fd handling: when -1, the backend treats the source/destination as
// single-buffer (Y + UV stacked in `fd` at offsets derived from wstride
// and height). This matches the rig dma_heap layout. When uv_fd is >= 0,
// the UV plane lives in a separate dma-buf — Y at offset 0 of `fd`, UV at
// offset 0 of `uv_fd`. This matches the host GBM split-buffer allocator,
// where the Y and UV planes are allocated as independent R8/GR88 BOs.
struct ConvertParams {
    int fd = -1;
    int uv_fd = -1; // -1 = same buffer as fd (single-bo, derive offset)
    PixelFormat fmt = PixelFormat::Nv12;
    int width = 0;
    int height = 0;
    int wstride = 0;    // Y-plane row stride (bytes); 0 → derive from width+fmt
    int hstride = 0;    // image height in lines; 0 → equals height
    int uv_wstride = 0; // UV-plane row stride (bytes); 0 → derive
    ColorSpace color_space = ColorSpace::Default; // read from dst param
};

[[nodiscard]] bool convert(const ConvertParams& src, const ConvertParams& dst);

} // namespace csc
