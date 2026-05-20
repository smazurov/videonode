// rga_csc — thin librga wrapper for dma-buf-to-dma-buf color-space
// conversion. The videonode-source sidecar uses it to turn HDMI-IN's
// native pixel formats (NV24 / NV16 / BGR3 / YUYV / UYVY) into the only
// format Mali-Panfrost will actually render: NV12.
//
// One Csc instance is reusable across frames — it imports src/dst handles
// per call (librga's importbuffer_fd dups the fd into its own ref).
//
// Cross-compat: builds against the rockchip-stubs on the dev host so the
// sidecar compiles everywhere. At runtime on the host the stub im* calls
// return non-success and convert() returns false — caller logs and exits.

#pragma once

// System path on rig (/usr/include/rga/im2d.h); resolved via
// third_party/rockchip-stubs/rga/ on host (rga_iface adds the include
// path only when stubs are in use, so real librga wins on rig).
// <cstddef> first: real /usr/include/rga/im2d_single.h uses NULL without
// including its definition itself.
#include <cstddef>
#include <rga/im2d.h>

#include <cstdint>

namespace rga {

// PixelFormat is our internal enum that hides the RK_FORMAT_* values.
// to_rk_format() does the mapping. Keeping our own enum means the rest of
// the codebase doesn't include librga headers transitively.
enum class PixelFormat {
    Nv12,
    Nv16,
    Nv24,
    Bgr3,
    Yuyv,
    Uyvy,
};

int to_rk_format(PixelFormat f);

// ConvertParams describes one source or destination buffer for convert().
// fd is the dma-buf fd (importbuffer_fd dups it; caller retains ownership).
// stride/height are the buffer's allocated stride and height — usually
// equal to width/height unless the producer added padding.
struct ConvertParams {
    int fd = -1;
    PixelFormat fmt = PixelFormat::Nv12;
    int width = 0;
    int height = 0;
    int wstride = 0; // bytes per row; 0 → derive from width+fmt
    int hstride = 0; // image height in lines; 0 → equals height
};

// convert() runs one imcvtcolor pass: src dma-buf -> dst dma-buf. Returns
// false on librga error (or on the host stub at runtime). Caller passes
// already-allocated buffers; this does not allocate.
bool convert(const ConvertParams& src, const ConvertParams& dst);

} // namespace rga
