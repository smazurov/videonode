// dmabuf_header — fixed-shape binary wire format for the SCM_RIGHTS
// dma-buf data plane. Replaces the legacy JSON-RPC envelope (see
// rpc/dmabuf_msg.{cpp,hpp} pre-cutover): same field set, packed into a
// little-endian byte sequence that's parsed in ~10 lines without any
// JSON dependency.
//
// Wire layout (all little-endian; producer and consumers are both
// composer/src/ binaries built from the same source):
//
//   offset  size  field
//     0      4    magic = 'D','B','U','F'  (0x46554244 LE)
//     4      2    version = 2
//     6      2    flags                    (reserved, must be 0)
//     8      4    slot_index
//    12      4    width
//    16      4    height
//    20      4    format_fourcc            (e.g. 'NV12' as 4 LE bytes)
//    24      8    frame_idx                (uint64 LE)
//    32      1    color_matrix             (enum)
//    33      1    color_range              (enum)
//    34      1    chroma_siting            (enum)
//    35      1    plane_count = N          (1..3)
//    36     4*N   plane_pitches[N]         (uint32 LE each)
//   36+4N   4*N   plane_offsets[N]         (uint32 LE each)
//   36+8N   8    generation               (uint64 LE; slot-reuse epoch)
//   total = 44 + 8N bytes
//
// Framing: no length prefix — the header is self-describing (peek
// plane_count after the fixed 36-byte prefix and read the rest). Magic
// catches desync, version field allows append-only evolution.
//
// `n_fds` is the SCM_RIGHTS ancillary count and is not duplicated in
// the header. Invariant (preserved verbatim from the legacy JSON
// codec): n_fds == plane_count for split-fd backends, or n_fds == 1
// with plane_count == 2 for single-fd NV12.

#pragma once

#include <cstdint>
#include <span>
#include <string>
#include <vector>

namespace dmabuf_header {

// Color contract enums mirror the legacy dmabuf_msg ones byte-for-byte
// so consumer code that switched on the old type still works after a
// drop-in rename. Producers must emit a non-Unspecified value for each
// field (CSC backend contract — RGA / GLES both converge on
// Bt601/Limited/Mpeg2 today).
enum class ColorMatrix : uint8_t {
    Unspecified = 0,
    Bt601 = 1,
    Bt709 = 2,
    Bt2020 = 3,
};

enum class ColorRange : uint8_t {
    Unspecified = 0,
    Limited = 1, // 16-235 luma / 16-240 chroma (broadcast)
    Full = 2,    // 0-255 (PC / JPEG)
};

enum class ChromaSiting : uint8_t {
    Unspecified = 0,
    Mpeg2 = 1, // chroma left-aligned to luma (H.264 / RGA)
    Jpeg = 2,  // chroma centered (MPEG-1 / JPEG)
};

constexpr uint32_t kMagic = 0x46554244; // 'D','B','U','F' little-endian
constexpr uint16_t kVersion = 2;
constexpr size_t kMaxPlanes = 3; // NV12 today, room for triplane

struct Header {
    uint32_t slot_index = 0;
    uint32_t width = 0;
    uint32_t height = 0;
    // 4-character DRM fourcc, e.g. "NV12". Stored as 4 raw bytes on the
    // wire; the string-typed field on the caller side preserves the
    // legacy ergonomic of writing `h.format = "NV12"`.
    std::string format;
    uint64_t frame_idx = 0;
    ColorMatrix color_matrix = ColorMatrix::Unspecified;
    ColorRange color_range = ColorRange::Unspecified;
    ChromaSiting chroma_siting = ChromaSiting::Unspecified;
    // Length must equal plane_count on the wire (1..kMaxPlanes).
    std::vector<uint32_t> plane_pitches;
    std::vector<uint32_t> plane_offsets;
    // Slot-reuse epoch. Consumer echoes (slot_index, generation) back as a
    // credit; the producer rejects a stale generation so a late credit can't
    // free a recycled slot.
    uint64_t generation = 0;
};

[[nodiscard]] std::vector<uint8_t> Encode(const Header& h);

// Unknown trailing bytes are tolerated (forward compat within the same version major).
[[nodiscard]] bool Decode(std::span<const uint8_t> bytes, Header& out, std::string* err = nullptr);

// SerializedSize returns the wire size of a header with the given
// plane_count. Useful for consumer-side recv buffer sizing.
[[nodiscard]] constexpr size_t SerializedSize(size_t plane_count) {
    return 44 + 8 * plane_count;
}

} // namespace dmabuf_header
