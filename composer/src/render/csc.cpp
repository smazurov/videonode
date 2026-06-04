#include "src/render/csc.hpp"

#include "src/common/log_levels.hpp"

#include <atomic>

#ifdef HAVE_RGA
#include "src/render/rga_csc.hpp"
#endif

#ifdef HAVE_PLACEBO_CSC
#include "src/render/csc_placebo.hpp"
#endif

namespace csc {

#ifdef HAVE_RGA
namespace {

rga::PixelFormat to_rga(PixelFormat f) {
    switch (f) {
    case PixelFormat::Nv12:
        return rga::PixelFormat::Nv12;
    case PixelFormat::Nv16:
        return rga::PixelFormat::Nv16;
    case PixelFormat::Nv24:
        return rga::PixelFormat::Nv24;
    case PixelFormat::Bgr3:
        return rga::PixelFormat::Bgr3;
    case PixelFormat::Bgra:
        return rga::PixelFormat::Bgra;
    case PixelFormat::Yuyv:
        return rga::PixelFormat::Yuyv;
    case PixelFormat::Uyvy:
        return rga::PixelFormat::Uyvy;
    }
    return rga::PixelFormat::Nv12;
}

rga::ColorSpace to_rga(ColorSpace c) {
    switch (c) {
    case ColorSpace::Bt709Limited:
        return rga::ColorSpace::Bt709Limited;
    case ColorSpace::Default:
        break;
    }
    return rga::ColorSpace::Default;
}

rga::ConvertParams to_rga(const ConvertParams& p) {
    rga::ConvertParams r;
    r.fd = p.fd;
    r.fmt = to_rga(p.fmt);
    r.width = p.width;
    r.height = p.height;
    // librga wants pixel stride; we carry bytes. Only 4-byte BGRA differs.
    r.wstride = (p.fmt == PixelFormat::Bgra && p.wstride > 0) ? p.wstride / 4 : p.wstride;
    r.hstride = p.hstride;
    r.color_space = to_rga(p.color_space);
    return r;
}

} // namespace
#endif

bool convert(const ConvertParams& src, const ConvertParams& dst) {
    // Backend priority: RGA on the rig (fixed-function, low latency),
    // GLES on Mesa boxes. The two never both compile in on the same host
    // in practice — librga only ships on RK3588 distros, libgbm always.
#ifdef HAVE_RGA
    return rga::convert(to_rga(src), to_rga(dst));
#elif defined(HAVE_PLACEBO_CSC)
    return csc_placebo::convert(src, dst);
#else
    static std::atomic<bool> warned{false};
    if (!warned.exchange(true)) {
        vn::log::warn("csc: no backend compiled in (HAVE_RGA off, HAVE_GLES_CSC off); convert() "
                      "returns false; non-NV12 frames will be dropped");
    }
    (void)src;
    (void)dst;
    return false;
#endif
}

} // namespace csc
