// mpp_jpeg_dec — Rockchip MPP hardware JPEG decoder wrapper.
//
// Why this exists: USB UVC cameras (like the Lyra) only deliver MJPEG at high
// resolutions because of USB bandwidth limits. We need NV12 in a dma-buf to
// feed GLES. MPP's MJPEG decoder runs on the RK3588 VDPU and produces NV12
// directly into an MPP-owned dma-buf — no CPU copy, no libturbojpeg dependency.
//
// Lifecycle model: decode_frame() returns a FrameRef holding a reference to
// an MPP-owned frame. The fd is valid until the FrameRef is destroyed or
// replaced. The decoder keeps a small pool of frame buffers internally, so
// the caller may hold one previous frame while submitting the next packet.
//
// Decode flow mirrors ffmpeg-rockchip's rkmppdec.c MJPEG path: two buffer
// groups (one for the input bitstream, one for output frames) and a one-shot
// info-change handshake (SET_EXT_BUF_GROUP / SET_PARSER_FAST_MODE /
// SET_INFO_CHANGE_READY) on the first decoded frame. Without that handshake
// the MJPEG hal leaves the parser in "info-change pending" and resets the
// jpegd block on every packet.
//
// Threading: each MppJpegDec instance is owned by exactly one capture
// thread; no internal synchronization. The FrameRef returned MUST be passed
// across thread boundaries with the same care as any std::unique_ptr.

#pragma once

#include "src/capture/jpeg_dec.hpp"

#include <cstddef>
#include <cstdint>
#include <memory>
#include <span>

// Gated on HAVE_MPP so dev hosts without librockchip_mpp can still parse
// the header (clang-tidy, IDE indexers) without bailing on missing
// <rockchip/rk_mpi.h>. Production rig builds set HAVE_MPP via
// composer/cmake/Dependencies.cmake; the matching .cpp is only compiled
// when HAVE_MPP is on.
#if defined(HAVE_MPP)

// rockchip-mpp declares MppCtx/MppApi/MppFrame as `typedef void*` (or
// equivalent), so we can't usefully forward-declare them. Just pull in the
// full MPP API surface; it's a clean C header.
#include <rockchip/rk_mpi.h>
#include <rockchip/mpp_frame.h>
#include <rockchip/mpp_buffer.h>
#include <rockchip/mpp_meta.h>

namespace mpp_jpeg_dec {

// RAII wrapper for one decoded MPP frame. Owns the underlying MppFrame
// reference and deinits it on destruction (returning the buffer to MPP's pool).
class FrameRef {
  public:
    FrameRef() = default;
    FrameRef(MppFrame f) : frame_(f) {}
    ~FrameRef() { reset(); }

    FrameRef(const FrameRef&) = delete;
    FrameRef& operator=(const FrameRef&) = delete;
    FrameRef(FrameRef&& other) noexcept : frame_(other.frame_) { other.frame_ = nullptr; }
    FrameRef& operator=(FrameRef&& other) noexcept {
        if (this != &other) {
            reset();
            frame_ = other.frame_;
            other.frame_ = nullptr;
        }
        return *this;
    }

    void reset();
    bool valid() const { return frame_ != nullptr; }
    explicit operator bool() const { return valid(); }

    // Access to frame metadata (only meaningful when valid()).
    int width() const;
    int height() const;
    int hor_stride() const; // padded stride, may exceed width()
    int ver_stride() const; // padded vert stride, may exceed height()
    int dmabuf_fd() const;  // dma-buf fd of the NV12 backing buffer
    MppFrameFormat fmt() const; // decoded subsampling (YUV420SP / 422SP / 444SP)
    uint32_t plane0_offset() const { return 0; }
    uint32_t plane0_pitch() const { return hor_stride(); }
    uint32_t plane1_offset() const { return hor_stride() * ver_stride(); } // NV12 UV starts after Y
    uint32_t plane1_pitch() const { return hor_stride(); }

  private:
    MppFrame frame_ = nullptr;
};

class MppJpegDec : public jpeg_dec::JpegDec {
  public:
    // Initialize the decoder. width/height size the output frame buffers; the
    // hal still reports the real geometry on the first frame's info-change.
    [[nodiscard]] bool init(int max_width, int max_height);
    ~MppJpegDec() override;

    MppJpegDec() = default;
    MppJpegDec(const MppJpegDec&) = delete;
    MppJpegDec& operator=(const MppJpegDec&) = delete;

    // Submit one full JPEG and synchronously wait for the decoded NV12 frame.
    // The returned FrameRef holds an MPP-owned dma-buf fd suitable for EGL
    // import / RGA / encoder input. Caller drops it (or assigns over) when
    // done. Returns an invalid FrameRef on failure.
    FrameRef decode(std::span<const uint8_t> jpeg);

    // jpeg_dec::JpegDec conformance. Holds the previously-decoded frame
    // internally (in pending_) so the dma-buf fd returned by the prior call
    // stays valid through the next broadcast — matches the TurboJPEG
    // backend's ping-pong semantics.
    [[nodiscard]] bool decode(std::span<const uint8_t> jpeg, jpeg_dec::DecodedNv12& out) override;

  private:
    // Tear down ctx + both buffer groups; safe to call partially-initialized.
    void teardown_();
    // Build the input bitstream packet (from buf_group_misc_). Returns nullptr
    // on failure. The returned packet owns its DRM buffer.
    [[nodiscard]] MppPacket alloc_input_packet_(std::span<const uint8_t> jpeg);
    // Attach an output NV12 frame to pkt via KEY_OUTPUT_FRAME meta. Returns the
    // frame (caller deinits on the error path) or nullptr on failure.
    [[nodiscard]] MppFrame alloc_output_frame_(MppPacket pkt);
    // Free the input packet MPP stashed in the output frame's KEY_INPUT_PACKET
    // meta (mirrors rkmppdec.c). Replaces the caller deiniting its own packet.
    void drain_input_packet_(MppFrame out);
    // One-shot info-change handshake on the first decoded frame: attach the
    // output group, enable fast parse, ack info-change. Returns false on a
    // control failure (caller drops the frame and retries next call).
    [[nodiscard]] bool handle_info_change_(MppFrame out);

    MppCtx ctx_ = nullptr;
    MppApi* mpi_ = nullptr;
    // Input bitstream + the first (pre-handshake) output frame come from the
    // misc group; real output frames come from buf_group_ once attached via
    // SET_EXT_BUF_GROUP. Both are DRM | DMA32 | CACHABLE.
    MppBufferGroup buf_group_misc_ = nullptr;
    MppBufferGroup buf_group_ = nullptr;
    int max_w_ = 0; // output buffer sizing (from init)
    int max_h_ = 0;
    bool cfg_done_ = false;
    bool info_change_done_ = false; // first-frame handshake latch
    uint64_t pts_ = 0;              // manufactured monotonic PTS for the hal
    uint64_t discard_count_ = 0;    // rate-limits the err/discard log
    FrameRef pending_;
};

} // namespace mpp_jpeg_dec

#endif // HAVE_MPP
