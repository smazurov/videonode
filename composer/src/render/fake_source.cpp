#include "src/render/fake_source.hpp"

#include "src/common/log_levels.hpp"

#include <algorithm>
#include <cstring>
#include <span>

namespace fake_source {

bool FakeSource::init(int width, int height, Color square_color, std::string_view heap_name) {
    if (width <= 0 || height <= 0 || (width & 1) || (height & 1)) {
        vn::log::error("fake_source: invalid dims %dx%d (must be even)", width, height);
        return false;
    }
    size_t size = static_cast<size_t>(width) * height * 3 / 2;
    buf_ = dmaheap::alloc(heap_name, size);
    if (!buf_.valid())
        return false;

    map_ = static_cast<uint8_t*>(dmaheap::mmap_rw(buf_));
    if (!map_)
        return false;

    w_ = width;
    h_ = height;
    color_ = square_color;

    // Initialize to all-black so the first frame is well-defined even before tick().
    dmaheap::sync_start(buf_.fd.get(), dmaheap::SyncDir::Write);
    std::memset(map_, 16, static_cast<size_t>(w_) * h_);                // Y plane = 16 (black)
    std::memset(map_ + w_ * h_, 128, static_cast<size_t>(w_) * h_ / 2); // UV plane = neutral
    dmaheap::sync_end(buf_.fd.get(), dmaheap::SyncDir::Write);
    return true;
}

FakeSource::~FakeSource() {
    if (map_)
        dmaheap::munmap_rw(map_, buf_.size);
}

namespace {

// Fill a rectangle in the Y plane with a given luma value. Clipped to image.
void fill_y(std::span<uint8_t> y_plane, int stride, int img_h, int x, int y, int w, int h,
            uint8_t value) {
    x = std::clamp(x, 0, stride);
    y = std::clamp(y, 0, img_h);
    int x2 = std::clamp(x + w, 0, stride);
    int y2 = std::clamp(y + h, 0, img_h);
    for (int row = y; row < y2; ++row) {
        std::memset(y_plane.data() + row * stride + x, value, static_cast<size_t>(x2 - x));
    }
}

// Fill a rectangle in the NV12 UV plane (interleaved CbCr) with given chroma.
// UV is half-resolution in both dimensions; one UV row covers two Y rows,
// and one UV pair (2 bytes) covers two Y columns. We just compute the
// half-coords and write in 2-byte pairs.
void fill_uv(std::span<uint8_t> uv_plane, int stride_y, int img_h_y, int x, int y, int w, int h,
             uint8_t u, uint8_t v) {
    int uv_stride = stride_y; // NV12 UV row stride matches Y row stride
    int uv_h = img_h_y / 2;
    int uvx = std::clamp(x / 2 * 2, 0, uv_stride);
    int uvy = std::clamp(y / 2, 0, uv_h);
    int uvx2 = std::clamp((x + w) / 2 * 2, 0, uv_stride);
    int uvy2 = std::clamp((y + h) / 2, 0, uv_h);
    for (int row = uvy; row < uvy2; ++row) {
        uint8_t* p = uv_plane.data() + row * uv_stride + uvx;
        for (int col = uvx; col < uvx2; col += 2) {
            *p++ = u;
            *p++ = v;
        }
    }
}

} // namespace

void FakeSource::tick(int frame_idx) {
    if (!map_)
        return;
    const size_t y_size = static_cast<size_t>(w_) * h_;
    const size_t uv_size = y_size / 2;
    std::span<uint8_t> y_plane(map_, y_size);
    std::span<uint8_t> uv_plane(map_ + y_size, uv_size);

    dmaheap::sync_start(buf_.fd.get(), dmaheap::SyncDir::Write);

    // Reset to black background.
    std::memset(y_plane.data(), 16, y_plane.size());
    std::memset(uv_plane.data(), 128, uv_plane.size());

    // Sweep a 200x200 colored square horizontally across the image, wrapping.
    // Period: w_ pixels at 4 px/frame -> w_/4 frames per sweep.
    constexpr int kSquare = 200;
    int sweep = w_ - kSquare;
    int sx = (frame_idx * 4) % (sweep > 0 ? sweep : 1);
    int sy = (h_ - kSquare) / 2;
    fill_y(y_plane, w_, h_, sx, sy, kSquare, kSquare, color_.y);
    fill_uv(uv_plane, w_, h_, sx, sy, kSquare, kSquare, color_.u, color_.v);

    // Frame-counter bar across the top: one tick per 30 frames (1 sec @ 30fps).
    // Lets us see at a glance that each source is animating at the right rate
    // without rendering text.
    int ticks = (frame_idx / 30) % 60;
    int bar_w = ticks * 16;
    fill_y(y_plane, w_, h_, 0, 0, bar_w, 24, color_.y);
    fill_uv(uv_plane, w_, h_, 0, 0, bar_w, 24, color_.u, color_.v);

    dmaheap::sync_end(buf_.fd.get(), dmaheap::SyncDir::Write);
}

} // namespace fake_source
