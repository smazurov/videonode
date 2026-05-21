#include "csc.hpp"

#include <atomic>
#include <cstdio>

#ifdef HAVE_RGA
#include "rga_csc.hpp"
#endif

#ifdef HAVE_GLES_CSC
#include "csc_gles.hpp"
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
    case PixelFormat::Yuyv:
        return rga::PixelFormat::Yuyv;
    case PixelFormat::Uyvy:
        return rga::PixelFormat::Uyvy;
    }
    return rga::PixelFormat::Nv12;
}

rga::ConvertParams to_rga(const ConvertParams& p) {
    rga::ConvertParams r;
    r.fd = p.fd;
    r.fmt = to_rga(p.fmt);
    r.width = p.width;
    r.height = p.height;
    r.wstride = p.wstride;
    r.hstride = p.hstride;
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
#elif defined(HAVE_GLES_CSC)
    return csc_gles::convert(src, dst);
#else
    static std::atomic<bool> warned{false};
    if (!warned.exchange(true)) {
        std::fprintf(stderr,
                     "csc: no backend compiled in (HAVE_RGA off, HAVE_GLES_CSC off); convert() "
                     "returns false; non-NV12 frames will be dropped\n");
    }
    (void)src;
    (void)dst;
    return false;
#endif
}

} // namespace csc
