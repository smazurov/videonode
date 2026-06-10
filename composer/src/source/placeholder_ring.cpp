#include "src/source/placeholder_ring.hpp"

#include "src/render/placeholder_painter.hpp"

#include <cstring>
#include <span>

namespace source {

bool PlaceholderRing::init(nv12_buf::Allocator& alloc, int w, int h,
                           const std::string& device_path) {
    width = w;
    height = h;
    const size_t tight = size_t(w) * h * 3 / 2;
    stage_.assign(tight, 0);
    if (!placeholder_painter::paint_base(stage_, w, h, device_path.c_str()))
        return false;
    std::span<const uint8_t> stage_span(stage_);
    auto y_plane = stage_span.first(size_t(w) * h);
    auto uv_plane = stage_span.subspan(size_t(w) * h);
    for (int i = 0; i < 2; ++i) {
        nv12_buf::Buffer b = alloc.alloc(w, h);
        if (!b.valid())
            return false;
        auto m = nv12_buf::map_rw(b);
        if (!m.y || !m.uv)
            return false;
        auto dst_y = m.y_bytes();
        auto dst_uv = m.uv_bytes();
        for (int y = 0; y < h; ++y) {
            std::memcpy(dst_y.subspan(size_t(y) * b.y_pitch, size_t(w)).data(),
                        y_plane.subspan(size_t(y) * w, size_t(w)).data(), size_t(w));
        }
        for (int y = 0; y < h / 2; ++y) {
            std::memcpy(dst_uv.subspan(size_t(y) * b.uv_pitch, size_t(w)).data(),
                        uv_plane.subspan(size_t(y) * w, size_t(w)).data(), size_t(w));
        }
        nv12_buf::unmap(b);
        bufs.push_back(std::move(b));
    }
    return true;
}

nv12_buf::Buffer& PlaceholderRing::paint_and_pick(uint64_t wallclock_ms, const char* status) {
    ++tick_idx;
    int idx = next;
    next = (next + 1) % int(bufs.size());
    (void)placeholder_painter::paint_tick(stage_, width, height, tick_idx, wallclock_ms, status);
    nv12_buf::Buffer& b = bufs[idx];
    auto m = nv12_buf::map_rw(b);
    if (m.y) {
        std::span<const uint8_t> stage_span(stage_);
        auto dst_y = m.y_bytes();
        for (int y = 0; y < height; ++y) {
            std::memcpy(dst_y.subspan(size_t(y) * b.y_pitch, size_t(width)).data(),
                        stage_span.subspan(size_t(y) * width, size_t(width)).data(), size_t(width));
        }
    }
    nv12_buf::unmap(b);
    return b;
}

void PlaceholderRing::destroy() {
    bufs.clear();
    stage_.clear();
}

} // namespace source
