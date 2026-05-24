// v4l2_format — format-negotiation methods of v4l2::Streamer.
// Split out of v4l2_capture.cpp for file-size budget; runtime ioctl
// wrappers stay in v4l2_capture.cpp.

#include "src/capture/v4l2_capture.hpp"

#include "src/common/log_levels.hpp"

#include <cerrno>
#include <cstring>
#include <linux/videodev2.h>
#include <sys/ioctl.h>

namespace v4l2 {

namespace {

// Retry-on-EINTR ioctl wrapper. Duplicated from v4l2_capture.cpp; the
// helper is 8 lines and stable, so a shared internal header would be
// overkill (see large-file-split-plan.md §4 Option A).
int xioctl(int fd, unsigned long req, void* arg) {
    int r;
    do {
        r = ::ioctl(fd, req, arg);
    } while (r < 0 && errno == EINTR);
    return r;
}

} // namespace

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
    } else {
        out.width = vfmt.fmt.pix.width;
        out.height = vfmt.fmt.pix.height;
        out.pixel_format = vfmt.fmt.pix.pixelformat;
    }
    out.fps = 0; // we don't query S_PARM here
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
