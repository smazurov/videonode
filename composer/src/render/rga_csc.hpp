#pragma once

// <cstddef> first: real /usr/include/rga/im2d_single.h uses NULL without
// including its definition itself.
#include <cstddef>
#include <cstdint>

#if defined(HAVE_RGA)
#include <rga/im2d.h>
#endif

namespace rga {

// PixelFormat is our internal enum that hides the RK_FORMAT_* values.
// to_rk_format() does the mapping. Keeping our own enum means the rest of
// the codebase doesn't include librga headers transitively.
enum class PixelFormat {
    Nv12,
    Nv16,
    Nv24,
    Bgr3,
    Bgra,
    Yuyv,
    Uyvy,
};

// Default maps to IM_COLOR_SPACE_DEFAULT (BT.601 limited for RGB→YUV).
enum class ColorSpace {
    Default,
    Bt709Limited,
};

#if defined(HAVE_RGA)
int to_rk_format(PixelFormat f);
#endif

// fd is the dma-buf fd (importbuffer_fd dups it; caller retains ownership).
// stride/height are the buffer's allocated stride and height — usually
// equal to width/height unless the producer added padding.
struct ConvertParams {
    int fd = -1;
    PixelFormat fmt = PixelFormat::Nv12;
    int width = 0;
    int height = 0;
    int wstride = 0;                              // pixels per row; 0 → derive from width
    int hstride = 0;                              // image height in lines; 0 → equals height
    ColorSpace color_space = ColorSpace::Default; // read from dst param
};

// Pure CSC (imcvtcolor) when dimensions match, resize+CSC (improcess) when
// they differ. Caller passes already-allocated buffers.
[[nodiscard]] bool convert(const ConvertParams& src, const ConvertParams& dst);

} // namespace rga
