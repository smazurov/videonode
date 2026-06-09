#include "src/render/csc.hpp"

#include "src/common/log_levels.hpp"

#include <array>
#include <atomic>
#include <cstddef>

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

#if defined(HAVE_RGA) && defined(HAVE_PLACEBO_CSC)

namespace {

constexpr std::size_t kFormatCount = static_cast<std::size_t>(PixelFormat::Uyvy) + 1;

const char* format_name(PixelFormat f) {
    switch (f) {
    case PixelFormat::Nv12:
        return "NV12";
    case PixelFormat::Nv16:
        return "NV16";
    case PixelFormat::Nv24:
        return "NV24";
    case PixelFormat::Bgr3:
        return "BGR3";
    case PixelFormat::Bgra:
        return "BGRA";
    case PixelFormat::Yuyv:
        return "YUYV";
    case PixelFormat::Uyvy:
        return "UYVY";
    }
    return "?";
}

enum class RgaState : uint8_t { Unknown, Usable, Unsupported };

} // namespace

bool convert(const ConvertParams& src, const ConvertParams& dst) {
    // RGA (fixed-function, low latency) first; fall back to the libplacebo GPU
    // backend for formats RGA can't convert (NV24 / YUV 4:4:4 on RK3588).
    // Latched per source-format so steady state never re-probes RGA or repeats
    // its failure log.
    static std::array<std::atomic<RgaState>, kFormatCount> states;
    const std::size_t idx = static_cast<std::size_t>(src.fmt);
    const RgaState s = states[idx].load(std::memory_order_relaxed);
    if (s != RgaState::Unsupported) {
        if (rga::convert(to_rga(src), to_rga(dst))) {
            if (s == RgaState::Unknown)
                states[idx].store(RgaState::Usable, std::memory_order_relaxed);
            return true;
        }
        if (s == RgaState::Usable)
            return false;
        states[idx].store(RgaState::Unsupported, std::memory_order_relaxed);
        vn::log::info("csc: RGA cannot convert %s; using libplacebo GPU fallback",
                      format_name(src.fmt));
    }
    return csc_placebo::convert(src, dst);
}

#elif defined(HAVE_RGA)

bool convert(const ConvertParams& src, const ConvertParams& dst) {
    return rga::convert(to_rga(src), to_rga(dst));
}

#elif defined(HAVE_PLACEBO_CSC)

bool convert(const ConvertParams& src, const ConvertParams& dst) {
    return csc_placebo::convert(src, dst);
}

#else

bool convert(const ConvertParams& src, const ConvertParams& dst) {
    static std::atomic<bool> warned{false};
    if (!warned.exchange(true)) {
        vn::log::warn("csc: no backend compiled in; convert() returns false; "
                      "non-NV12 frames will be dropped");
    }
    (void)src;
    (void)dst;
    return false;
}

#endif

} // namespace csc
