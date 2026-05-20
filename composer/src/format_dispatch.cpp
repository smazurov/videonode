#include "format_dispatch.hpp"

#include <drm_fourcc.h>

namespace format_dispatch {

uint32_t fourcc_from_string(std::string_view s) {
    if (s.size() != 4)
        return 0;
    return uint32_t(uint8_t(s[0])) | (uint32_t(uint8_t(s[1])) << 8) |
           (uint32_t(uint8_t(s[2])) << 16) | (uint32_t(uint8_t(s[3])) << 24);
}

void fill_image_desc(egl_ctx::EglCtx::ImageDesc& d, std::string_view fmt, uint32_t width,
                     uint32_t height) {
    uint32_t fourcc = fourcc_from_string(fmt);
    if (fourcc == 0)
        fourcc = DRM_FORMAT_NV12; // back-compat default
    d.fourcc = fourcc;

    if (d.plane0_pitch == 0) {
        // Luma row = width for semi-planar; for packed RGB the caller is
        // expected to set plane0_pitch explicitly (= width * bytes/pixel).
        d.plane0_pitch = int(width);
    }

    // If producer already populated plane1, trust them.
    if (d.plane1_pitch != 0 || d.plane1_offset != 0)
        return;

    // Derive plane1 for known semi-planar layouts. Chroma plane sits
    // immediately after luma in the same dma-buf.
    switch (fourcc) {
    case DRM_FORMAT_NV12:
        d.plane1_offset = int(width * height);
        d.plane1_pitch = int(width); // CbCr at half W, half H
        break;
    case DRM_FORMAT_NV16:
        d.plane1_offset = int(width * height);
        d.plane1_pitch = int(width); // CbCr at half W, full H
        break;
    case DRM_FORMAT_NV24:
        d.plane1_offset = int(width * height);
        d.plane1_pitch = int(width * 2); // CbCr at full W, full H
        break;
    default:
        // Single-plane formats (BGR888, RGB888, etc.) keep plane1_* = 0.
        break;
    }
}

} // namespace format_dispatch
