#include "src/ipc/dmabuf_header.hpp"

#include <cstddef>
#include <cstdint>
#include <span>
#include <string>

namespace {

bool equal(const dmabuf_header::Header& a, const dmabuf_header::Header& b) {
    return a.slot_index == b.slot_index && a.width == b.width && a.height == b.height &&
           a.format == b.format && a.frame_idx == b.frame_idx && a.color_matrix == b.color_matrix &&
           a.color_range == b.color_range && a.chroma_siting == b.chroma_siting &&
           a.plane_pitches == b.plane_pitches && a.plane_offsets == b.plane_offsets &&
           a.generation == b.generation;
}

} // namespace

extern "C" int LLVMFuzzerTestOneInput(const uint8_t* data, size_t size) {
    dmabuf_header::Header out;
    std::string err;
    if (!dmabuf_header::Decode(std::span<const uint8_t>(data, size), out, &err)) {
        return 0;
    }

    // Oracle: anything Decode accepts must survive a re-encode/re-decode
    // round trip unchanged. Catches lossy or non-canonical field handling
    // that pure memory-safety (ASan/UBSan) can't see.
    dmabuf_header::Header again;
    if (!dmabuf_header::Decode(dmabuf_header::Encode(out), again, nullptr) || !equal(out, again)) {
        __builtin_trap();
    }
    return 0;
}
