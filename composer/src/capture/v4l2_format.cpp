// v4l2_format — format-negotiation methods of v4l2::Streamer.
// Split out of v4l2_capture.cpp for file-size budget; runtime ioctl
// wrappers stay in v4l2_capture.cpp.

#include "src/capture/v4l2_capture.hpp"

#include "src/capture/v4l2_fps.hpp"
#include "src/common/log_levels.hpp"

#include <cerrno>
#include <cstring>
#include <linux/videodev2.h>
#include <sys/ioctl.h>

namespace v4l2 {

uint32_t pix_fmt_from_name(const std::string& name) {
    if (name == "NV12")
        return V4L2_PIX_FMT_NV12;
    if (name == "NV16")
        return V4L2_PIX_FMT_NV16;
    if (name == "NV24")
        return V4L2_PIX_FMT_NV24;
    if (name == "BGR3" || name == "BG24")
        return V4L2_PIX_FMT_BGR24;
    if (name == "YUYV")
        return V4L2_PIX_FMT_YUYV;
    if (name == "UYVY")
        return V4L2_PIX_FMT_UYVY;
    if (name == "MJPG" || name == "MJPEG")
        return V4L2_PIX_FMT_MJPEG;
    if (name.size() != 4)
        return 0;
    return uint32_t(uint8_t(name[0])) | (uint32_t(uint8_t(name[1])) << 8) |
           (uint32_t(uint8_t(name[2])) << 16) | (uint32_t(uint8_t(name[3])) << 24);
}

namespace {

// Retry-on-EINTR ioctl wrapper. Duplicated from v4l2_capture.cpp; the
// helper is 8 lines and stable, so a shared internal header would be
// overkill.
int xioctl(int fd, unsigned long req, void* arg) {
    int r;
    do {
        r = ::ioctl(fd, req, arg);
    } while (r < 0 && errno == EINTR);
    return r;
}

} // namespace

ColorMatrix resolve_matrix(uint32_t colorspace, uint32_t ycbcr_enc, uint32_t height) {
    if (ycbcr_enc == V4L2_YCBCR_ENC_709)
        return ColorMatrix::Bt709;
    if (ycbcr_enc == V4L2_YCBCR_ENC_601)
        return ColorMatrix::Bt601;
    switch (colorspace) {
    case V4L2_COLORSPACE_REC709:
        return ColorMatrix::Bt709;
    case V4L2_COLORSPACE_SMPTE170M:
    case V4L2_COLORSPACE_470_SYSTEM_M:
    case V4L2_COLORSPACE_470_SYSTEM_BG:
        return ColorMatrix::Bt601;
    default:
        return height >= 720 ? ColorMatrix::Bt709 : ColorMatrix::Bt601;
    }
}

bool Streamer::get_format(StreamFormat& out) const {
    if (fd_ < 0) {
        errno = EBADF;
        return false;
    }
    v4l2_format vfmt{};
    vfmt.type = buf_type_();
    if (xioctl(fd_, VIDIOC_G_FMT, &vfmt) < 0) {
        vn::log::error("v4l2_capture: VIDIOC_G_FMT: %s", strerror(errno));
        return false;
    }
    if (multiplanar_) {
        out.width = vfmt.fmt.pix_mp.width;
        out.height = vfmt.fmt.pix_mp.height;
        out.pixel_format = vfmt.fmt.pix_mp.pixelformat;
        out.color_matrix =
            resolve_matrix(vfmt.fmt.pix_mp.colorspace, vfmt.fmt.pix_mp.ycbcr_enc, out.height);
    } else {
        out.width = vfmt.fmt.pix.width;
        out.height = vfmt.fmt.pix.height;
        out.pixel_format = vfmt.fmt.pix.pixelformat;
        out.color_matrix =
            resolve_matrix(vfmt.fmt.pix.colorspace, vfmt.fmt.pix.ycbcr_enc, out.height);
    }
    out.fps = query_capture_fps(xioctl, fd_, buf_type_());
    return true;
}

bool Streamer::set_format(const StreamFormat& f) {
    if (fd_ < 0) {
        errno = EBADF;
        return false;
    }
    v4l2_format vfmt{};
    vfmt.type = buf_type_();
    if (multiplanar_) {
        vfmt.fmt.pix_mp.width = f.width;
        vfmt.fmt.pix_mp.height = f.height;
        vfmt.fmt.pix_mp.pixelformat = f.pixel_format;
        vfmt.fmt.pix_mp.field = V4L2_FIELD_NONE;
    } else {
        vfmt.fmt.pix.width = f.width;
        vfmt.fmt.pix.height = f.height;
        vfmt.fmt.pix.pixelformat = f.pixel_format;
        vfmt.fmt.pix.field = V4L2_FIELD_NONE;
    }
    if (xioctl(fd_, VIDIOC_S_FMT, &vfmt) < 0) {
        vn::log::error("v4l2_capture: VIDIOC_S_FMT: %s", strerror(errno));
        return false;
    }

    if (f.fps != 0) {
        v4l2_streamparm parm{};
        parm.type = buf_type_();
        parm.parm.capture.timeperframe.numerator = 1;
        parm.parm.capture.timeperframe.denominator = f.fps;
        if (xioctl(fd_, VIDIOC_S_PARM, &parm) < 0) {
            // Many drivers (rk_hdmirx) silently ignore S_PARM; log + continue.
            vn::log::warn("v4l2_capture: VIDIOC_S_PARM ignored: %s", strerror(errno));
        }
    }
    return true;
}

bool Streamer::query_dv_timings_valid() const {
    if (fd_ < 0) {
        errno = EBADF;
        return false;
    }
    v4l2_dv_timings t{};
    if (xioctl(fd_, VIDIOC_QUERY_DV_TIMINGS, &t) < 0)
        return false;
    return t.bt.width > 0 && t.bt.height > 0;
}

Streamer::DvTimingsState Streamer::query_dv_timings_state() const {
    if (fd_ < 0)
        return DvTimingsState::OtherError;
    v4l2_dv_timings t{};
    if (xioctl(fd_, VIDIOC_QUERY_DV_TIMINGS, &t) == 0) {
        if (t.bt.width > 0 && t.bt.height > 0 && t.bt.pixelclock > 0) {
            return DvTimingsState::Locked;
        }
        return DvTimingsState::Unstable;
    }
    switch (errno) {
    case ENOLINK:
        return DvTimingsState::NoLink;
    case ENOLCK:
        return DvTimingsState::Unstable;
    case ERANGE:
        return DvTimingsState::OutOfRange;
    case ENOTTY:
        return DvTimingsState::NotSupported;
    default:
        return DvTimingsState::OtherError;
    }
}

} // namespace v4l2
