#pragma once

#include "src/render/nv12_buf.hpp"

#include <cstdint>
#include <string>
#include <vector>

namespace source {

struct PlaceholderRing {
    int width = 0;
    int height = 0;
    std::vector<nv12_buf::Buffer> bufs;
    std::vector<uint8_t> stage_;
    int next = 0;
    uint64_t tick_idx = 0;

    [[nodiscard]] bool init(nv12_buf::Allocator& alloc, int w, int h,
                            const std::string& device_path);
    [[nodiscard]] nv12_buf::Buffer& paint_and_pick(uint64_t wallclock_ms, const char* status);
    void destroy();
};

} // namespace source
