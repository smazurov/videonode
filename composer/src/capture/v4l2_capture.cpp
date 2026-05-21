#include "src/capture/v4l2_capture.hpp"

#include <cerrno>
#include <cstdio>
#include <cstring>
#include <fcntl.h>
#include <linux/videodev2.h>
#include <poll.h>
#include <sys/ioctl.h>
#include <sys/mman.h>
#include <unistd.h>

namespace v4l2 {

namespace {

// Retry-on-EINTR ioctl wrapper. Mirrors Go's ioctl helper.
int xioctl(int fd, unsigned long req, void* arg) {
    int r;
    do {
        r = ::ioctl(fd, req, arg);
    } while (r < 0 && errno == EINTR);
    return r;
}

void close_planes(BufferRef& b) {
    for (auto& p : b.planes) {
        if (p.dma_buf_fd >= 0) {
            ::close(p.dma_buf_fd);
            p.dma_buf_fd = -1;
        }
    }
}

} // namespace

bool Streamer::open(const std::string& device_path) {
    fd_ = ::open(device_path.c_str(), O_RDWR | O_NONBLOCK | O_CLOEXEC);
    if (fd_ < 0) {
        fprintf(stderr, "v4l2_capture: open(%s): %s\n", device_path.c_str(), strerror(errno));
        return false;
    }
    device_path_ = device_path;

    v4l2_capability cap{};
    if (xioctl(fd_, VIDIOC_QUERYCAP, &cap) < 0) {
        fprintf(stderr, "v4l2_capture: VIDIOC_QUERYCAP %s: %s\n", device_path.c_str(),
                strerror(errno));
        close();
        return false;
    }
    // Prefer device_caps when available (newer drivers); fall back to caps.
    uint32_t caps = (cap.capabilities & V4L2_CAP_DEVICE_CAPS) ? cap.device_caps : cap.capabilities;
    bool single = caps & V4L2_CAP_VIDEO_CAPTURE;
    bool multi = caps & V4L2_CAP_VIDEO_CAPTURE_MPLANE;
    if (!single && !multi) {
        fprintf(stderr, "v4l2_capture: %s: neither single- nor multi-plane capture\n",
                device_path.c_str());
        close();
        return false;
    }
    multiplanar_ = multi;
    return true;
}

void Streamer::unmap_all_() {
    for (auto& m : in_maps_) {
        if (m.first && m.first != MAP_FAILED)
            ::munmap(m.first, m.second);
    }
    in_maps_.clear();
}

void Streamer::close() {
    if (streaming_)
        (void)stream_off();
    unmap_all_();
    for (auto& b : bufs_)
        close_planes(b);
    bufs_.clear();
    if (fd_ >= 0) {
        ::close(fd_);
        fd_ = -1;
    }
    streaming_ = false;
}

Streamer::~Streamer() {
    close();
}

uint32_t Streamer::buf_type_() const {
    return multiplanar_ ? V4L2_BUF_TYPE_VIDEO_CAPTURE_MPLANE : V4L2_BUF_TYPE_VIDEO_CAPTURE;
}

bool Streamer::get_format(StreamFormat& out) const {
    if (fd_ < 0) {
        errno = EBADF;
        return false;
    }
    v4l2_format vfmt{};
    vfmt.type = buf_type_();
    if (xioctl(fd_, VIDIOC_G_FMT, &vfmt) < 0) {
        fprintf(stderr, "v4l2_capture: VIDIOC_G_FMT: %s\n", strerror(errno));
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
        fprintf(stderr, "v4l2_capture: VIDIOC_S_FMT: %s\n", strerror(errno));
        return false;
    }

    if (f.fps != 0) {
        v4l2_streamparm parm{};
        parm.type = buf_type_();
        parm.parm.capture.timeperframe.numerator = 1;
        parm.parm.capture.timeperframe.denominator = f.fps;
        if (xioctl(fd_, VIDIOC_S_PARM, &parm) < 0) {
            // Many drivers (rk_hdmirx) silently ignore S_PARM; log + continue.
            fprintf(stderr, "v4l2_capture: VIDIOC_S_PARM ignored: %s\n", strerror(errno));
        }
    }
    return true;
}

bool Streamer::request_buffers(int count, std::vector<BufferRef>& out) {
    if (fd_ < 0) {
        errno = EBADF;
        return false;
    }
    // If we've allocated before, close the old fds and drop the old mmaps
    // before re-requesting — the kernel reassigns offsets on each REQBUFS.
    unmap_all_();
    for (auto& b : bufs_)
        close_planes(b);
    bufs_.clear();

    v4l2_requestbuffers req{};
    req.count = static_cast<uint32_t>(count);
    req.type = buf_type_();
    req.memory = V4L2_MEMORY_MMAP;
    if (xioctl(fd_, VIDIOC_REQBUFS, &req) < 0) {
        fprintf(stderr, "v4l2_capture: VIDIOC_REQBUFS count=%d: %s\n", count, strerror(errno));
        return false;
    }
    if (req.count == 0) {
        fprintf(stderr, "v4l2_capture: driver refused buffer allocation\n");
        return false;
    }

    bufs_.reserve(req.count);
    for (uint32_t i = 0; i < req.count; ++i) {
        BufferRef b;
        if (!query_buffer_(i, b))
            return false;
        bufs_.push_back(std::move(b));
    }
    out = bufs_; // copy refs; dma_buf_fd is still -1 until export_buffer
    return true;
}

bool Streamer::query_buffer_(uint32_t index, BufferRef& out) {
    v4l2_buffer buf{};
    v4l2_plane planes[VIDEO_MAX_PLANES]{};
    buf.type = buf_type_();
    buf.memory = V4L2_MEMORY_MMAP;
    buf.index = index;
    if (multiplanar_) {
        buf.m.planes = planes;
        buf.length = VIDEO_MAX_PLANES;
    }
    if (xioctl(fd_, VIDIOC_QUERYBUF, &buf) < 0) {
        fprintf(stderr, "v4l2_capture: VIDIOC_QUERYBUF index=%u: %s\n", index, strerror(errno));
        return false;
    }
    out.index = index;
    if (multiplanar_) {
        out.length = 0;
        out.planes.resize(buf.length);
        for (uint32_t p = 0; p < buf.length; ++p) {
            out.planes[p].length = planes[p].length;
            out.planes[p].mmap_offset = planes[p].m.mem_offset;
            out.planes[p].dma_buf_fd = -1;
            out.length += planes[p].length;
        }
    } else {
        out.length = buf.length;
        out.planes.resize(1);
        out.planes[0].length = buf.length;
        out.planes[0].mmap_offset = buf.m.offset;
        out.planes[0].dma_buf_fd = -1;
    }
    return true;
}

bool Streamer::export_buffer(uint32_t index, uint32_t plane, int& out_fd) {
    if (fd_ < 0) {
        errno = EBADF;
        return false;
    }
    v4l2_exportbuffer ex{};
    ex.type = buf_type_();
    ex.index = index;
    ex.plane = plane;
    ex.flags = O_RDWR | O_CLOEXEC;
    if (xioctl(fd_, VIDIOC_EXPBUF, &ex) < 0) {
        fprintf(stderr, "v4l2_capture: VIDIOC_EXPBUF index=%u plane=%u: %s\n", index, plane,
                strerror(errno));
        return false;
    }
    out_fd = ex.fd;
    if (index < bufs_.size() && plane < bufs_[index].planes.size()) {
        bufs_[index].planes[plane].dma_buf_fd = ex.fd;
    }
    return true;
}

bool Streamer::export_all_planes() {
    for (auto& b : bufs_) {
        for (uint32_t p = 0; p < b.planes.size(); ++p) {
            int fd = -1;
            if (!export_buffer(b.index, p, fd))
                return false;
        }
    }
    return true;
}

bool Streamer::queue_buffer(uint32_t index) {
    if (fd_ < 0) {
        errno = EBADF;
        return false;
    }
    v4l2_buffer buf{};
    v4l2_plane planes[VIDEO_MAX_PLANES]{};
    buf.type = buf_type_();
    buf.memory = V4L2_MEMORY_MMAP;
    buf.index = index;
    if (multiplanar_) {
        buf.m.planes = planes;
        buf.length = static_cast<uint32_t>(bufs_[index].planes.size());
        for (uint32_t p = 0; p < buf.length; ++p) {
            planes[p].length = bufs_[index].planes[p].length;
        }
    }
    if (xioctl(fd_, VIDIOC_QBUF, &buf) < 0) {
        fprintf(stderr, "v4l2_capture: VIDIOC_QBUF index=%u: %s\n", index, strerror(errno));
        return false;
    }
    return true;
}

bool Streamer::dequeue_buffer(int timeout_ms, DequeuedFrame& out) {
    if (fd_ < 0) {
        errno = EBADF;
        return false;
    }
    pollfd pfd{fd_, short(POLLIN | POLLPRI), 0};
    int pr = ::poll(&pfd, 1, timeout_ms);
    if (pr <= 0) {
        if (pr == 0)
            errno = ETIMEDOUT;
        return false;
    }
    if (pfd.revents & POLLPRI) {
        (void)drain_events();
    }
    if (!(pfd.revents & POLLIN)) {
        errno = EAGAIN;
        return false;
    }
    v4l2_buffer buf{};
    v4l2_plane planes[VIDEO_MAX_PLANES]{};
    buf.type = buf_type_();
    buf.memory = V4L2_MEMORY_MMAP;
    if (multiplanar_) {
        buf.m.planes = planes;
        buf.length = VIDEO_MAX_PLANES;
    }
    if (xioctl(fd_, VIDIOC_DQBUF, &buf) < 0) {
        return false;
    }
    out.index = buf.index;
    out.bytesused = buf.bytesused;
    out.sequence = buf.sequence;
    out.timestamp_ns =
        uint64_t(buf.timestamp.tv_sec) * 1000000000ULL + uint64_t(buf.timestamp.tv_usec) * 1000ULL;
    out.flags = buf.flags;
    if (multiplanar_) {
        out.plane_bytesused.resize(buf.length);
        out.bytesused = 0;
        for (uint32_t p = 0; p < buf.length; ++p) {
            out.plane_bytesused[p] = planes[p].bytesused;
            out.bytesused += planes[p].bytesused;
        }
    } else {
        out.plane_bytesused.clear();
    }
    return true;
}

bool Streamer::stream_on() {
    if (fd_ < 0) {
        errno = EBADF;
        return false;
    }
    uint32_t type = buf_type_();
    if (xioctl(fd_, VIDIOC_STREAMON, &type) < 0) {
        fprintf(stderr, "v4l2_capture: VIDIOC_STREAMON: %s\n", strerror(errno));
        return false;
    }
    streaming_ = true;
    return true;
}

bool Streamer::subscribe_ctrl_event(uint32_t cid) {
    if (fd_ < 0) {
        errno = EBADF;
        return false;
    }
    v4l2_event_subscription sub{};
    sub.type = V4L2_EVENT_CTRL;
    sub.id = cid;
    if (xioctl(fd_, VIDIOC_SUBSCRIBE_EVENT, &sub) < 0)
        return false;
    return true;
}

bool Streamer::drain_events_typed(std::vector<v4l2_event>& out) {
    if (fd_ < 0) {
        errno = EBADF;
        return false;
    }
    for (;;) {
        v4l2_event ev{};
        if (xioctl(fd_, VIDIOC_DQEVENT, &ev) < 0) {
            if (errno == ENOENT)
                return true;
            return false;
        }
        out.push_back(ev);
    }
}

bool Streamer::read_ctrl(uint32_t cid, int32_t& out_value) const {
    if (fd_ < 0) {
        errno = EBADF;
        return false;
    }
    v4l2_control c{};
    c.id = cid;
    if (xioctl(fd_, VIDIOC_G_CTRL, &c) < 0)
        return false;
    out_value = c.value;
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

bool Streamer::subscribe_source_change() {
    if (fd_ < 0) {
        errno = EBADF;
        return false;
    }
    v4l2_event_subscription sub{};
    sub.type = V4L2_EVENT_SOURCE_CHANGE;
    if (xioctl(fd_, VIDIOC_SUBSCRIBE_EVENT, &sub) < 0) {
        fprintf(stderr, "v4l2_capture: VIDIOC_SUBSCRIBE_EVENT: %s\n", strerror(errno));
        return false;
    }
    return true;
}

bool Streamer::drain_events(bool* drained) {
    if (drained)
        *drained = false;
    if (fd_ < 0) {
        errno = EBADF;
        return false;
    }
    for (;;) {
        v4l2_event ev{};
        if (xioctl(fd_, VIDIOC_DQEVENT, &ev) < 0) {
            if (errno == ENOENT)
                return true;
            return false;
        }
        if (drained)
            *drained = true;
        fprintf(stderr, "v4l2_capture: drained event type=0x%x seq=%u\n", ev.type, ev.sequence);
    }
}

bool Streamer::restart_streaming() {
    if (!stream_off())
        return false;
    for (const auto& b : bufs_) {
        if (!queue_buffer(b.index))
            return false;
    }
    return stream_on();
}

bool Streamer::stream_off() {
    if (fd_ < 0 || !streaming_)
        return true;
    uint32_t type = buf_type_();
    if (xioctl(fd_, VIDIOC_STREAMOFF, &type) < 0) {
        fprintf(stderr, "v4l2_capture: VIDIOC_STREAMOFF: %s\n", strerror(errno));
        return false;
    }
    streaming_ = false;
    return true;
}

std::optional<std::span<std::byte>> Streamer::mmap_buffer_span(uint32_t index) {
    if (fd_ < 0) {
        errno = EBADF;
        return std::nullopt;
    }
    if (multiplanar_) {
        fprintf(stderr, "v4l2_capture: mmap_buffer_span not supported on multiplanar device\n");
        errno = ENOTSUP;
        return std::nullopt;
    }
    if (index >= bufs_.size() || bufs_[index].planes.empty()) {
        errno = EINVAL;
        return std::nullopt;
    }
    const PlaneRef& p = bufs_[index].planes[0];
    if (p.length == 0) {
        errno = EINVAL;
        return std::nullopt;
    }
    void* m = ::mmap(nullptr, p.length, PROT_READ, MAP_SHARED, fd_, p.mmap_offset);
    if (m == MAP_FAILED) {
        fprintf(stderr, "v4l2_capture: mmap index=%u: %s\n", index, strerror(errno));
        return std::nullopt;
    }
    in_maps_.emplace_back(m, p.length);
    return std::span<std::byte>(static_cast<std::byte*>(m), p.length);
}

} // namespace v4l2
