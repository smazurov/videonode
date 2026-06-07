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
    // Per out_ring slot reuse epoch, stamped into the dma-buf header so a
    // consumer's credit can be matched to the exact generation it read.
    std::vector<uint64_t> out_ring_gen;
    // Frames dropped because every ring slot was still held by a consumer.
    uint64_t out_ring_drops = 0;
    csc::PixelFormat src_fmt = csc::PixelFormat::Nv12;
    std::string src_fmt_name;
    int width = 0;
    int height = 0;
    uint32_t fps = 0; // actual negotiated capture rate (VIDIOC_G_PARM)
    v4l2::ColorMatrix color_matrix = v4l2::ColorMatrix::Bt601;

    DecodeMode mode = DecodeMode::Rga;

    std::unique_ptr<jpeg_dec::JpegDec> jpeg;
    bool using_mpp = false; // log-only
    // MPP HW decode passes 4:2:2 / 4:4:4 sources through as NV16 / NV24. Those
    // need a CSC pass to NV12 before broadcast, into this out_ring. Allocated
    // lazily on the first non-NV12 frame so the common 4:2:0 camera pays
    // nothing; latched here so it only allocates once per session.
    bool mpp_csc_ring_ready = false;
    std::vector<void*> in_maps;
    std::vector<size_t> in_map_sizes;
    // TurboJPEG decode writes NV12 directly into the bo: per-slot Y/UV
    // mmap pointers obtained from nv12_buf::map_rw. Held across the
    // session; nv12_buf::unmap() runs in teardown_session_.
    std::vector<void*> out_y;
    std::vector<void*> out_uv;
};

uint32_t v4l2_pix_fmt_(const std::string& s);

void teardown_session_(CaptureSession& s);

// Depth matches the RGA path (a.buffers + 3).
[[nodiscard]] bool ensure_mpp_output_ring(CaptureSession& s, const Args& a,
                                          nv12_buf::Allocator& allocator);

// Outcome of try_open_capture, classified from the open() errno so the
// reopen loop can map each case to the right health/liveness:
//   Ok     — device open + streaming.
//   Absent — node gone (ENOENT/ENODEV); the device is unplugged.
//   Busy   — node present but not yet usable (EBUSY/EACCES); udev settling.
//   Failed — any other open errno, or a later negotiation step failing.
enum class CaptureOpenStatus { Ok, Absent, Busy, Failed };

// quiet=true on the 1 Hz reopen loop suppresses the per-attempt open() error log.
[[nodiscard]] CaptureOpenStatus try_open_capture(CaptureSession& s, const Args& a,
                                                 nv12_buf::Allocator& allocator,
                                                 bool quiet = false);

} // namespace source
