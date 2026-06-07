#pragma once

#include <cstdint>
#include <optional>

namespace source {

// reserve_out_slot scans the output ring from `start` (inclusive, wrapping) and
// returns the first slot for which is_free(slot) holds. Returns nullopt after a
// full lap with none free — the caller drops the frame rather than overwrite a
// slot a consumer still holds.
template <typename Pred>
[[nodiscard]] std::optional<uint32_t> reserve_out_slot(uint32_t ring_size, uint32_t start,
                                                       Pred is_free) {
    if (ring_size == 0)
        return std::nullopt;
    for (uint32_t i = 0; i < ring_size; ++i) {
        uint32_t slot = (start + i) % ring_size;
        if (is_free(slot))
            return slot;
    }
    return std::nullopt;
}

} // namespace source
