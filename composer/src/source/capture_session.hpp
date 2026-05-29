// CaptureSession — V4L2 streamer + decoder + NV12 output ring. Internal
// header for the source/ library; the bin/ entry point doesn't see it.
#pragma once

#include "src/capture/jpeg_dec.hpp"
#include "src/capture/v4l2_capture.hpp"
#include "src/render/csc.hpp"
#include "src/render/nv12_buf.hpp"
#include "src/source/orchestrator.hpp"

#include <memory>
#include <string>
#include <vector>

namespace source {

enum class DecodeMode {
    Rga,   // RGA color-space-convert raw V4L2 format → NV12 into out_ring
    Mjpeg, // JPEG decode (MPP HW on rig, TurboJPEG SW on host) → NV12
};

// CaptureSession bundles V4L2 streamer + decoder + output buffers.
//
// RGA path: out_ring is filled by RGA on each DQBUF.
// MJPEG / MPP backend: out_ring is unused (MPP owns its pool).
// MJPEG / TurboJPEG backend: out_ring is mmap'd writable (out_maps) and
//                            the decoder writes NV12 bytes directly into
//                            the next slot.
struct CaptureSession {
    bool active = false;
    v4l2::Streamer cap;
    std::vector<nv12_buf::Buffer> out_ring;
    uint32_t out_ring_write = 0;
    csc::PixelFormat src_fmt = csc::PixelFormat::Nv12;
    std::string src_fmt_name;
    int width = 0;
    int height = 0;
    uint32_t fps = 0; // actual negotiated capture rate (VIDIOC_G_PARM)

    DecodeMode mode = DecodeMode::Rga;

    // MJPEG path:
    std::unique_ptr<jpeg_dec::JpegDec> jpeg;
    bool using_mpp = false;     // log-only
    std::vector<void*> in_maps; // V4L2 capture buffer mmaps (JPEG bytes)
    std::vector<size_t> in_map_sizes;
    // TurboJPEG decode writes NV12 directly into the bo: per-slot Y/UV
    // mmap pointers obtained from nv12_buf::map_rw. Held across the
    // session; nv12_buf::unmap() runs in teardown_session_.
    std::vector<void*> out_y;
    std::vector<void*> out_uv;
};

// FOURCC → V4L2 pixfmt code. Returns 0 for unknown / malformed strings.
uint32_t v4l2_pix_fmt_(const std::string& s);

// Release every fd/mmap held by `s` and reset it to a fresh state.
void teardown_session_(CaptureSession& s);

// Open the V4L2 device, negotiate format, request buffers, build the
// decoder (RGA or JPEG), and stream on. Returns false on any failure
// (with `s` torn down). Reuses `allocator` for the NV12 output ring.
bool try_open_capture(CaptureSession& s, const Args& a, nv12_buf::Allocator& allocator);

} // namespace source
