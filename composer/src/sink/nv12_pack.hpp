#pragma once

#include "src/ipc/scm_rights_source.hpp"

#include <cstdint>
#include <span>
#include <vector>

namespace sink {

// Reusable 16-byte-aligned buffer for SSE streaming stores.
class AlignedScratch {
  public:
    [[nodiscard]] std::span<uint8_t> get(size_t bytes);

  private:
    std::vector<uint8_t> buf_;
};

// mmaps the producer's NV12 dma-buf(s) and packs Y then UV rows tightly
// (stride padding removed). Returns an empty span when the frame is unusable.
[[nodiscard]] std::span<const uint8_t> pack_nv12(const scm_rights_source::FrameView& v,
                                                 AlignedScratch& scratch);

[[nodiscard]] bool write_full(int fd, std::span<const uint8_t> buf);

} // namespace sink
