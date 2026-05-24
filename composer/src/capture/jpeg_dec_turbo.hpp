// jpeg_dec_turbo — libjpeg-turbo (TurboJPEG API) software MJPEG → NV12
// decoder. Used by videonode-source as the host-build path when Rockchip
// MPP isn't available.
//
// Output model: the caller pre-allocates a small ring of NV12 dma-buf
// slots and hands the per-slot (Y fd, Y mmap, UV fd, UV mmap) tuples to
// init(). decode() ping-pongs across the ring so the previously-broadcast
// slot stays untouched while the next frame writes — consumers reading
// the prior fds see a stable image.
//
// Two slot shapes are accepted:
//   - Contiguous (rig dma_heap): y_fd == uv_fd, uv_mapped == y_mapped + W*H.
//   - Split (Fedora GBM, radeonsi import constraint): y_fd != uv_fd, the two
//     mmaps live in independent regions. The interleave loop writes to
//     uv_mapped directly, so it doesn't care which shape it got.
//
// Limitations: only 4:2:0 and 4:2:2 baseline JPEG are accepted (UVC
// virtually always produces 4:2:0). Other subsamplings would need a
// chroma resample step we don't bother with; decode() rejects them so the
// source falls back to placeholder rather than corrupting output.

#pragma once

#include "src/capture/jpeg_dec.hpp"

#include <cstdint>
#include <vector>

namespace jpeg_dec {

class TurboJpegDec : public JpegDec {
  public:
    struct Slot {
        int y_fd = -1;
        int uv_fd = -1;               // == y_fd for contiguous; separate dma-buf on GBM split
        uint8_t* y_mapped = nullptr;  // PROT_READ|PROT_WRITE, W*H bytes
        uint8_t* uv_mapped = nullptr; // PROT_READ|PROT_WRITE, W*H/2 bytes
    };

    TurboJpegDec() = default;
    ~TurboJpegDec() override;
    TurboJpegDec(const TurboJpegDec&) = delete;
    TurboJpegDec& operator=(const TurboJpegDec&) = delete;

    // Initialize for fixed-geometry output. ring must be non-empty (2 is
    // the practical minimum — one slot for the broadcast, one for the
    // next decode). Returns false on tjInitDecompress() failure or empty
    // ring.
    [[nodiscard]] bool init(int width, int height, std::vector<Slot> ring);

    [[nodiscard]] bool decode(std::span<const uint8_t> jpeg, DecodedNv12& out) override;

  private:
    void* handle_ = nullptr; // tjhandle (kept across frames)
    int width_ = 0;
    int height_ = 0;
    std::vector<Slot> ring_;
    std::size_t next_ = 0;
    // Chroma scratch — TurboJPEG decompresses to planar I420; we own the
    // interleave into NV12's UV plane. Sized lazily on first decode().
    std::vector<uint8_t> u_scratch_;
    std::vector<uint8_t> v_scratch_;
};

} // namespace jpeg_dec
