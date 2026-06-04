#include "src/render/rga_csc.hpp"

#include "src/common/log_levels.hpp"

// Gated on HAVE_RGA so the file can sit in compile_commands.json even on
// dev hosts where librga is absent — clang-tidy / IDE indexers still parse
// it without bailing out on missing RK_FORMAT_* / IM_STATUS / rga_buffer_*
// identifiers. The CMake rule (composer/src/render/CMakeLists.txt) only
// links this TU when HAVE_RGA is on, so the empty translation unit on
// non-rig builds is benign.
#if defined(HAVE_RGA)

namespace rga {

int to_rk_format(PixelFormat f) {
    switch (f) {
    case PixelFormat::Nv12:
        return RK_FORMAT_YCbCr_420_SP;
    case PixelFormat::Nv16:
        return RK_FORMAT_YCbCr_422_SP;
    case PixelFormat::Nv24:
        return RK_FORMAT_YCbCr_444_SP;
    case PixelFormat::Bgr3:
        return RK_FORMAT_BGR_888;
    case PixelFormat::Bgra:
        return RK_FORMAT_BGRA_8888;
    case PixelFormat::Yuyv:
        return RK_FORMAT_YUYV_422;
    case PixelFormat::Uyvy:
        return RK_FORMAT_UYVY_422;
    }
    return -1;
}

namespace {

// Default wstride (in PIXELS — librga uses pixel-stride regardless of
// bytes-per-pixel; we verified this against the kernel error "Only get
// buffer X byte but required Y byte" where Y == width*3*height*3 because
// we'd passed byte stride for BGR3). For tightly packed formats this is
// just the image width.
int default_wstride(PixelFormat fmt, int width) {
    (void)fmt;
    return width;
}

struct ImportedBuffer {
    rga_buffer_handle_t handle = 0;
    rga_buffer_t buf{};

    [[nodiscard]] bool valid() const { return handle != 0; }
};

// Spelling must match the rig's /usr/include/rga/im2d_type.h (rig-verify).
int to_color_mode(ColorSpace cs) {
    if (cs == ColorSpace::Bt709Limited)
        return IM_RGB_TO_YUV_BT709_LIMIT;
    return IM_COLOR_SPACE_DEFAULT;
}

ImportedBuffer import_(const ConvertParams& p) {
    ImportedBuffer ib;
    int wstride = p.wstride > 0 ? p.wstride : default_wstride(p.fmt, p.width);
    int hstride = p.hstride > 0 ? p.hstride : p.height;
    im_handle_param_t hp{};
    hp.width = p.width;
    hp.height = p.height;
    hp.format = to_rk_format(p.fmt);
    ib.handle = importbuffer_fd(p.fd, &hp);
    if (ib.handle == 0) {
        vn::log::error("rga_csc: importbuffer_fd failed (fd=%d fmt=%d w=%d h=%d)", p.fd, hp.format,
                       p.width, p.height);
        return ib;
    }
    ib.buf = wrapbuffer_handle_t(ib.handle, p.width, p.height, wstride, hstride, hp.format);
    return ib;
}

} // namespace

bool convert(const ConvertParams& src, const ConvertParams& dst) {
    if (src.width != dst.width || src.height != dst.height) {
        // RGA scale+CSC (improcess) not yet wired; libplacebo handles
        // downscale today. Output defaults to canvas dims, so this is unhit.
        vn::log::error("rga_csc: scaling convert not implemented (%dx%d -> %dx%d)", src.width,
                       src.height, dst.width, dst.height);
        return false;
    }
    ImportedBuffer sb = import_(src);
    if (!sb.valid())
        return false;
    ImportedBuffer db = import_(dst);
    if (!db.valid()) {
        releasebuffer_handle(sb.handle);
        return false;
    }

    IM_STATUS st = imcvtcolor(sb.buf, db.buf, to_rk_format(src.fmt), to_rk_format(dst.fmt),
                              to_color_mode(dst.color_space));

    releasebuffer_handle(sb.handle);
    releasebuffer_handle(db.handle);

    if (st != IM_STATUS_SUCCESS && st != IM_STATUS_NOERROR) {
        vn::log::error("rga_csc: imcvtcolor failed (status=%d)", static_cast<int>(st));
        return false;
    }
    return true;
}

} // namespace rga

#endif // HAVE_RGA
