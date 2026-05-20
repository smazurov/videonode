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
// Threading: each MppJpegDec instance is owned by exactly one capture
// thread; no internal synchronization. The FrameRef returned MUST be passed
// across thread boundaries with the same care as any std::unique_ptr.

#pragma once

#include <cstddef>
#include <cstdint>
#include <memory>

// rockchip-mpp declares MppCtx/MppApi/MppFrame as `typedef void*` (or
// equivalent), so we can't usefully forward-declare them. Just pull in the
// full MPP API surface; it's a clean C header.
#include <rockchip/rk_mpi.h>
#include <rockchip/mpp_frame.h>
#include <rockchip/mpp_buffer.h>

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
    uint32_t plane0_offset() const { return 0; }
    uint32_t plane0_pitch() const { return hor_stride(); }
    uint32_t plane1_offset() const { return hor_stride() * ver_stride(); } // NV12 UV starts after Y
    uint32_t plane1_pitch() const { return hor_stride(); }

  private:
    MppFrame frame_ = nullptr;
};

class MppJpegDec {
  public:
    // Initialize the decoder. width/height are hints to size the buffer pool;
    // MPP picks its own internal pool sizes but knowing the input dims up
    // front saves an info-change round-trip on the first frame.
    bool init(int max_width, int max_height);
    ~MppJpegDec();

    MppJpegDec() = default;
    MppJpegDec(const MppJpegDec&) = delete;
    MppJpegDec& operator=(const MppJpegDec&) = delete;

    // Submit one full JPEG and synchronously wait for the decoded NV12 frame.
    // The returned FrameRef holds an MPP-owned dma-buf fd suitable for EGL
    // import / RGA / encoder input. Caller drops it (or assigns over) when
    // done. Returns an invalid FrameRef on failure.
    FrameRef decode(const uint8_t* jpeg_data, size_t jpeg_size);

  private:
    MppCtx ctx_ = nullptr;
    MppApi* mpi_ = nullptr;
    bool cfg_done_ = false;
};

} // namespace mpp_jpeg_dec
