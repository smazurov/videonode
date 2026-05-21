// jpeg_dec — abstract MJPEG → NV12 decode interface.
//
// Two backends conform to this:
//   - mpp_jpeg_dec::MppJpegDec — Rockchip MPP HW decode (rig). Output fd is
//     an MPP-pool dma-buf with padded hor_stride / ver_stride.
//   - jpeg_dec::TurboJpegDec  — libjpeg-turbo software decode (host). Output
//     fd is one of the caller's pre-allocated NV12 dma-heap slots; pitches
//     equal width (tight NV12).
//
// videonode-source probes MPP first, falls through to TurboJPEG on init
// failure. Both produce the same DecodedNv12 shape so the main loop and
// SCM_RIGHTS broadcast helper don't need to know which backend ran.

#pragma once

#include <cstddef>
#include <cstdint>
#include <span>

namespace jpeg_dec {

// One decoded NV12 frame. The dma-buf fd is owned by the backend — for
// MPP it points into the decoder's pool, for TurboJPEG into the caller's
// out_ring slot. Either way, the fd stays valid until the NEXT decode()
// call on the same backend (each backend holds one frame's worth of
// previous-tick stability).
struct DecodedNv12 {
    int fd = -1;
    int plane1_fd = -1; // optional: separate UV fd (Fedora GBM split-buffer); -1 means reuse fd
    int width = 0;
    int height = 0;
    uint32_t y_pitch = 0;
    uint32_t uv_pitch = 0;
    uint32_t y_offset = 0;
    uint32_t uv_offset = 0;
};

class JpegDec {
  public:
    virtual ~JpegDec() = default;

    // Decode one full JPEG. Returns false on header mismatch, unsupported
    // subsampling, or decode error (each backend logs to stderr).
    virtual bool decode(const uint8_t* jpeg, std::size_t size, DecodedNv12& out) = 0;

    bool decode(std::span<const uint8_t> jpeg, DecodedNv12& out) {
        return decode(jpeg.data(), jpeg.size(), out);
    }
};

} // namespace jpeg_dec
