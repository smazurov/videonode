// fake_source — synthetic NV12 frame producer backed by a dma_heap buffer.
//
// Each FakeSource owns one DMA-BUF holding W*H*3/2 bytes (NV12: Y plane + UV
// interleaved plane). tick() animates the contents in place. The dma-buf fd
// is what we hand to EGL (for GLES sampling) and to RGA / ffmpeg downstream.
//
// Why dma_heap-backed instead of just malloc'd memory:
//   - GLES can sample it directly via EGL_LINUX_DMA_BUF_EXT (zero-copy import).
//   - librga can imcvtcolor/imblend it without CPU staging.
//   - It matches what real V4L2 capture (EXPBUF) will produce in the
//     production daemon, so the rest of the pipeline never knows the source
//     is synthetic.
//
// The animation is intentionally cheap (a moving colored square on a black
// background, plus a tiny frame counter LED bar) so the CPU cost is
// negligible at canvas fps. Per-source color makes it obvious in ffplay
// whether each quad is animating independently.

#pragma once

#include "src/ipc/dma_heap.hpp"

#include <cstdint>

namespace fake_source {

struct Color {
    uint8_t y, u, v;
};

// A few preset colors that survive the YUV→RGB round-trip cleanly enough
// to be unambiguous in ffplay. BT.601 limited range.
inline constexpr Color kRed = {.y = 76, .u = 85, .v = 255};
inline constexpr Color kGreen = {.y = 149, .u = 43, .v = 21};
inline constexpr Color kBlue = {.y = 29, .u = 255, .v = 107};
inline constexpr Color kYellow = {.y = 225, .u = 0, .v = 148};
inline constexpr Color kCyan = {.y = 178, .u = 171, .v = 0};
inline constexpr Color kWhite = {.y = 235, .u = 128, .v = 128};

class FakeSource {
  public:
    // Allocate the dma-buf and mmap it. Returns false on failure.
    // width and height must be even (NV12 chroma subsampling).
    [[nodiscard]] bool init(int width, int height, Color square_color,
                            std::string_view heap_name = "system");

    // Update the buffer contents for the given frame index. Cheap: writes
    // a black background and a 200x200 colored square at a position that
    // sweeps horizontally over ~3 seconds at 30fps.
    void tick(int frame_idx);

    [[nodiscard]] int dmabuf_fd() const { return buf_.fd.get(); }
    int width() const { return w_; }
    int height() const { return h_; }
    size_t size() const { return buf_.size; }

    ~FakeSource();
    FakeSource() = default;
    FakeSource(const FakeSource&) = delete;
    FakeSource& operator=(const FakeSource&) = delete;

  private:
    dmaheap::Buffer buf_;
    uint8_t* map_ = nullptr;
    int w_ = 0;
    int h_ = 0;
    Color color_ = kWhite;
};

} // namespace fake_source
