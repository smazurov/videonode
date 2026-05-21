// csc — backend-agnostic color-space-conversion API for the dma-buf
// pipeline. videonode-source calls into here for every V4L2 frame whose
// native format isn't already NV12.
//
// One backend is selected at build time:
//   - HAVE_RGA       → Rockchip RGA (librga) on RK3588.
//   - HAVE_GLES_CSC  → Mesa EGL/GLES MRT shader on generic Linux.
//
// If neither is compiled in, convert() logs once and returns false; the
// caller drops the frame.
//
// All backends MUST produce the contract declared in dmabuf_msg.hpp:
// BT.601 limited range with MPEG-2 chroma siting.

#pragma once

#include <cstdint>

namespace csc {

// PixelFormat is the input/output format of one convert() call. Mirrors
// the V4L2 four-cc set this pipeline handles. Output is always Nv12.
enum class PixelFormat {
    Nv12,
    Nv16,
    Nv24,
    Bgr3,
    Yuyv,
    Uyvy,
};

// ConvertParams describes one source or destination dma-buf. fd is borrowed
// — the backend may dup it internally but does not take ownership.
struct ConvertParams {
    int fd = -1;
    PixelFormat fmt = PixelFormat::Nv12;
    int width = 0;
    int height = 0;
    int wstride = 0; // bytes per row; 0 → derive from width+fmt
    int hstride = 0; // image height in lines; 0 → equals height
};

// convert runs one src→dst conversion using the active backend. Returns
// false on backend error or when no backend is compiled in.
bool convert(const ConvertParams& src, const ConvertParams& dst);

} // namespace csc
