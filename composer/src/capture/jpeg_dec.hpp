// jpeg_dec — abstract MJPEG → NV12 decode interface.
//
// Two backends conform to this:
//   - mpp_jpeg_dec::MppJpegDec — Rockchip MPP HW decode (rig). Output fd is
//     an MPP-pool dma-buf with padded hor_stride / ver_stride.
//   - jpeg_dec::TurboJpegDec  — libjpeg-turbo software decode (host). Output
//     fd is one of the caller's pre-allocated NV12 slots; pitches are
//     forwarded from the slot (== width on rig dma_heap, >= width on
//     GBM-allocated R8/GR88 BOs that pad for alignment).
//
// videonode-source probes MPP first, falls through to TurboJPEG on init
// failure. Both produce the same DecodedNv12 shape so the main loop and
// SCM_RIGHTS broadcast helper don't need to know which backend ran.

#pragma once

#include <cstddef>
#include <cstdint>
#include <span>

namespace jpeg_dec {

// Chroma subsampling the backend actually decoded into. TurboJPEG always
// downconverts to Nv12; MPP HW decode passes through the JPEG's native
// subsampling, so a 4:2:2 / 4:4:4 source surfaces as Nv16 / Nv24 here. The
// orchestrator runs a CSC pass to NV12 for the non-Nv12 cases before broadcast
// (the wire/sink/snapshot contract is NV12-only). Kept local to jpeg_dec so the
// base capture interface stays independent of render/csc.hpp.
enum class PixelFormat { Nv12, Nv16, Nv24 };

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
    // Producer ring slot + reuse epoch carried into the dma-buf header for
    // the consumer credit back-channel. UINT32_MAX = not ring-backed (MJPEG
    // pool / placeholder; no recycle hazard).
    uint32_t slot_index = 0xFFFFFFFFu;
    uint64_t generation = 0;
    // Subsampling the backend decoded into. Nv16 / Nv24 mean the planes carry
    // 4:2:2 / 4:4:4 chroma at the geometry described above and must be CSC'd to
    // NV12 before broadcast; Nv12 frames broadcast zero-copy.
    PixelFormat pixel_format = PixelFormat::Nv12;
};

class JpegDec {
  public:
    virtual ~JpegDec() = default;

    // Decode one full JPEG. Returns false on header mismatch, unsupported
    // subsampling, or decode error (each backend logs to stderr).
    [[nodiscard]] virtual bool decode(std::span<const uint8_t> jpeg, DecodedNv12& out) = 0;
};

} // namespace jpeg_dec
