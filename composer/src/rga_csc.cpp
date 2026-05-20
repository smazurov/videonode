#include "rga_csc.hpp"

#include <cstdio>

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

    bool valid() const { return handle != 0; }
};

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
        fprintf(stderr, "rga_csc: importbuffer_fd failed (fd=%d fmt=%d w=%d h=%d)\n", p.fd,
                hp.format, p.width, p.height);
        return ib;
    }
    ib.buf = wrapbuffer_handle_t(ib.handle, p.width, p.height, wstride, hstride, hp.format);
    return ib;
}

} // namespace

bool convert(const ConvertParams& src, const ConvertParams& dst) {
    ImportedBuffer sb = import_(src);
    if (!sb.valid())
        return false;
    ImportedBuffer db = import_(dst);
    if (!db.valid()) {
        releasebuffer_handle(sb.handle);
        return false;
    }

    IM_STATUS st = imcvtcolor(sb.buf, db.buf, to_rk_format(src.fmt), to_rk_format(dst.fmt),
                              IM_COLOR_SPACE_DEFAULT);

    releasebuffer_handle(sb.handle);
    releasebuffer_handle(db.handle);

    if (st != IM_STATUS_SUCCESS && st != IM_STATUS_NOERROR) {
        fprintf(stderr, "rga_csc: imcvtcolor failed (status=%d)\n", static_cast<int>(st));
        return false;
    }
    return true;
}

} // namespace rga
