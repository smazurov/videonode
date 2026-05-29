// CaptureSession implementation. See capture_session.hpp.

#include "src/source/capture_session.hpp"

#include "src/common/log_keys.hpp"
#include "src/common/log_levels.hpp"

#include "src/capture/jpeg_dec_turbo.hpp"
#if defined(HAVE_MPP)
#include "src/capture/mpp_jpeg_dec.hpp"
#endif

#include <cerrno>
#include <cstdio>
#include <cstdint>
#include <linux/videodev2.h>
#include <memory>
#include <vector>

namespace source {

namespace {

uint32_t fourcc_(const std::string& s) {
    if (s.size() != 4)
        return 0;
    return uint32_t(uint8_t(s[0])) | (uint32_t(uint8_t(s[1])) << 8) |
           (uint32_t(uint8_t(s[2])) << 16) | (uint32_t(uint8_t(s[3])) << 24);
}

bool v4l2_to_csc_(uint32_t v4l2_fmt, csc::PixelFormat& out, std::string& name) {
    switch (v4l2_fmt) {
    case V4L2_PIX_FMT_NV12:
        out = csc::PixelFormat::Nv12;
        name = "NV12";
        return true;
    case V4L2_PIX_FMT_NV16:
        out = csc::PixelFormat::Nv16;
        name = "NV16";
        return true;
    case V4L2_PIX_FMT_NV24:
        out = csc::PixelFormat::Nv24;
        name = "NV24";
        return true;
    case V4L2_PIX_FMT_BGR24:
        out = csc::PixelFormat::Bgr3;
        name = "BGR3";
        return true;
    case V4L2_PIX_FMT_YUYV:
        out = csc::PixelFormat::Yuyv;
        name = "YUYV";
        return true;
    case V4L2_PIX_FMT_UYVY:
        out = csc::PixelFormat::Uyvy;
        name = "UYVY";
        return true;
    }
    return false;
}

bool maybe_renegotiate_to_rga_friendly_(v4l2::Streamer& cap, v4l2::StreamFormat& cur) {
    if (cur.pixel_format != V4L2_PIX_FMT_NV24)
        return false;
    v4l2::StreamFormat want = cur;
    want.pixel_format = V4L2_PIX_FMT_NV16;
    if (!cap.set_format(want))
        return false;
    return cap.get_format(cur);
}

// Negotiate format and determine decode mode. On success, sets s.width,
// s.height, s.mode, s.src_fmt_name (and s.src_fmt for RGA path).
// Returns false and closes cap on failure.
bool negotiate_format_(CaptureSession& s, const Args& a) {
    v4l2::StreamFormat cur;
    if (a.in_format.empty()) {
        if (!s.cap.get_format(cur)) {
            s.cap.close();
            return false;
        }
        maybe_renegotiate_to_rga_friendly_(s.cap, cur);
    } else {
        cur.pixel_format = v4l2_pix_fmt_(a.in_format);
        cur.width = a.in_width > 0 ? a.in_width : a.placeholder_w;
        cur.height = a.in_height > 0 ? a.in_height : a.placeholder_h;
        cur.fps = a.in_fps;
        if (!s.cap.set_format(cur)) {
            s.cap.close();
            return false;
        }
        if (!s.cap.get_format(cur)) {
            s.cap.close();
            return false;
        }
    }

    if (cur.pixel_format == V4L2_PIX_FMT_MJPEG) {
        s.mode = DecodeMode::Mjpeg;
        s.src_fmt_name = "MJPG";
    } else {
        s.mode = DecodeMode::Rga;
        if (!v4l2_to_csc_(cur.pixel_format, s.src_fmt, s.src_fmt_name)) {
            s.cap.close();
            return false;
        }
    }
    s.width = int(cur.width);
    s.height = int(cur.height);
    s.fps = cur.fps; // actual negotiated rate from VIDIOC_G_PARM
    if (s.width <= 0 || s.height <= 0) {
        s.cap.close();
        return false;
    }
    return true;
}

// Request buffers, export DMA-BUF planes, and queue all buffers for
// streaming. Returns false and closes cap on failure.
bool allocate_and_queue_buffers_(CaptureSession& s, const Args& a) {
    std::vector<v4l2::BufferRef> _ignored;
    if (!s.cap.request_buffers(a.buffers, _ignored)) {
        s.cap.close();
        return false;
    }
    if (!s.cap.export_all_planes()) {
        s.cap.close();
        return false;
    }
    for (const auto& b : s.cap.buffers()) {
        if (!s.cap.queue_buffer(b.index)) {
            s.cap.close();
            return false;
        }
    }
    if (!s.cap.stream_on()) {
        s.cap.close();
        return false;
    }
    return true;
}

// Set up the MJPEG decode path: mmap capture buffers for CPU JPEG reads,
// probe MPP or fall back to TurboJPEG, populate s.jpeg + s.out_ring.
bool setup_mjpeg_decoder_(CaptureSession& s, const Args& a, nv12_buf::Allocator& allocator) {
    for (const auto& b : s.cap.buffers()) {
        auto mapped = s.cap.mmap_buffer_span(b.index);
        if (!mapped) {
            s.cap.close();
            return false;
        }
        s.in_maps.push_back(mapped->data());
        s.in_map_sizes.push_back(mapped->size());
    }

    std::unique_ptr<jpeg_dec::JpegDec> mpp;
#if defined(HAVE_MPP)
    auto mpp_concrete = std::make_unique<mpp_jpeg_dec::MppJpegDec>();
    if (mpp_concrete->init(s.width, s.height))
        mpp = std::move(mpp_concrete);
#endif
    if (mpp) {
        s.jpeg = std::move(mpp);
        s.using_mpp = true;
        vn::log::info("videonode-source: MJPEG backend = MPP (HW)");
        return true;
    }

    // TurboJPEG fallback: allocate out_ring + map each slot for CPU writes.
    for (int i = 0; i < a.buffers; ++i) {
        nv12_buf::Buffer b = allocator.alloc(s.width, s.height);
        if (!b.valid()) {
            s.cap.close();
            s.out_ring.clear();
            return false;
        }
        s.out_ring.push_back(std::move(b));
    }
    std::vector<jpeg_dec::TurboJpegDec::Slot> slots;
    slots.reserve(s.out_ring.size());
    for (auto& buf : s.out_ring) {
        auto m = nv12_buf::map_rw(buf);
        if (!m.y || !m.uv) {
            vn::log::error("videonode-source: map_rw out_ring y_fd=%d uv_fd=%d failed (y=%p uv=%p)",
                           buf.y_fd, buf.uv_fd, m.y, m.uv);
            s.cap.close();
            return false;
        }
        s.out_y.push_back(m.y);
        s.out_uv.push_back(m.uv);
        slots.push_back({buf.y_fd, buf.uv_fd, static_cast<uint8_t*>(m.y),
                         static_cast<uint8_t*>(m.uv), buf.y_pitch, buf.uv_pitch});
    }
    auto tj = std::make_unique<jpeg_dec::TurboJpegDec>();
    if (!tj->init(s.width, s.height, std::move(slots))) {
        s.cap.close();
        return false;
    }
    s.jpeg = std::move(tj);
    s.using_mpp = false;
    vn::log::info("videonode-source: MJPEG backend = TurboJPEG (SW)");
    return true;
}

// Set up the RGA/GLES CSC output ring (NV12 buffers the CSC writes into).
bool setup_rga_output_ring_(CaptureSession& s, const Args& a, nv12_buf::Allocator& allocator) {
    int ring_depth = a.buffers + 1;
    for (int i = 0; i < ring_depth; ++i) {
        nv12_buf::Buffer b = allocator.alloc(s.width, s.height);
        if (!b.valid()) {
            s.cap.close();
            s.out_ring.clear();
            return false;
        }
        s.out_ring.push_back(std::move(b));
    }
    return true;
}

} // namespace

uint32_t v4l2_pix_fmt_(const std::string& s) {
    if (s == "NV12")
        return V4L2_PIX_FMT_NV12;
    if (s == "NV16")
        return V4L2_PIX_FMT_NV16;
    if (s == "NV24")
        return V4L2_PIX_FMT_NV24;
    if (s == "BGR3" || s == "BG24")
        return V4L2_PIX_FMT_BGR24;
    if (s == "YUYV")
        return V4L2_PIX_FMT_YUYV;
    if (s == "UYVY")
        return V4L2_PIX_FMT_UYVY;
    if (s == "MJPG" || s == "MJPEG")
        return V4L2_PIX_FMT_MJPEG;
    return fourcc_(s);
}

void teardown_session_(CaptureSession& s) {
    if (s.jpeg)
        s.jpeg.reset();
    for (auto& b : s.out_ring)
        nv12_buf::unmap(b);
    s.out_y.clear();
    s.out_uv.clear();
    s.in_maps.clear();
    s.in_map_sizes.clear();
    s.out_ring.clear();
    s.cap.close(); // unmaps V4L2 in_maps inside Streamer
    s.active = false;
    s.width = 0;
    s.height = 0;
    s.src_fmt_name.clear();
    s.mode = DecodeMode::Rga;
    s.using_mpp = false;
}

CaptureOpenStatus try_open_capture(CaptureSession& s, const Args& a, nv12_buf::Allocator& allocator,
                                   bool quiet) {
    teardown_session_(s);
    if (!s.cap.open(a.device, quiet)) {
        // Classify the open() errno (preserved by quiet mode) so the reopen
        // loop can pick the right liveness: absent (unplugged) vs busy
        // (udev settling) vs a real fault.
        switch (errno) {
        case ENOENT:
        case ENODEV:
            return CaptureOpenStatus::Absent;
        case EBUSY:
        case EACCES:
            return CaptureOpenStatus::Busy;
        default:
            return CaptureOpenStatus::Failed;
        }
    }
    if (!negotiate_format_(s, a))
        return CaptureOpenStatus::Failed;
    if (!allocate_and_queue_buffers_(s, a))
        return CaptureOpenStatus::Failed;
    if (s.mode == DecodeMode::Mjpeg) {
        if (!setup_mjpeg_decoder_(s, a, allocator))
            return CaptureOpenStatus::Failed;
    } else {
        if (!setup_rga_output_ring_(s, a, allocator))
            return CaptureOpenStatus::Failed;
    }
    s.active = true;
    char w_s[16], h_s[16], buf_s[16];
    std::snprintf(w_s, sizeof(w_s), "%d", s.width);
    std::snprintf(h_s, sizeof(h_s), "%d", s.height);
    std::snprintf(buf_s, sizeof(buf_s), "%d", int(s.cap.buffers().size()));
    const char* mode =
        s.mode == DecodeMode::Mjpeg ? (s.using_mpp ? "mjpeg-mpp" : "mjpeg-turbojpeg") : "rga";
    vn::log::info_s("videonode-source: capture ready",
                    {vn::key::width, w_s, vn::key::height, h_s, vn::key::fourcc,
                     s.src_fmt_name.c_str(), vn::key::buffers, buf_s, vn::key::mode, mode});
    return CaptureOpenStatus::Ok;
}

} // namespace source
