#include "src/ipc/dmabuf_header.hpp"

#include <cstring>

namespace dmabuf_header {

namespace {

void put_u32_le(std::vector<uint8_t>& out, uint32_t v) {
    out.push_back(static_cast<uint8_t>(v & 0xff));
    out.push_back(static_cast<uint8_t>((v >> 8) & 0xff));
    out.push_back(static_cast<uint8_t>((v >> 16) & 0xff));
    out.push_back(static_cast<uint8_t>((v >> 24) & 0xff));
}

void put_u16_le(std::vector<uint8_t>& out, uint16_t v) {
    out.push_back(static_cast<uint8_t>(v & 0xff));
    out.push_back(static_cast<uint8_t>((v >> 8) & 0xff));
}

void put_u64_le(std::vector<uint8_t>& out, uint64_t v) {
    for (int shift = 0; shift < 64; shift += 8) {
        out.push_back(static_cast<uint8_t>((v >> shift) & 0xff));
    }
}

uint32_t get_u32_le(const uint8_t* p) {
    return static_cast<uint32_t>(p[0]) | (static_cast<uint32_t>(p[1]) << 8) |
           (static_cast<uint32_t>(p[2]) << 16) | (static_cast<uint32_t>(p[3]) << 24);
}

uint16_t get_u16_le(const uint8_t* p) {
    return static_cast<uint16_t>(p[0]) | static_cast<uint16_t>(p[1] << 8);
}

uint64_t get_u64_le(const uint8_t* p) {
    uint64_t v = 0;
    for (int shift = 0; shift < 64; shift += 8) {
        v |= static_cast<uint64_t>(*p++) << shift;
    }
    return v;
}

bool set_err(std::string* err, const char* msg) {
    if (err)
        *err = msg;
    return false;
}

} // namespace

std::vector<uint8_t> Encode(const Header& h) {
    const size_t plane_count = h.plane_pitches.size();
    std::vector<uint8_t> out;
    out.reserve(SerializedSize(plane_count));

    put_u32_le(out, kMagic);
    put_u16_le(out, kVersion);
    put_u16_le(out, 0); // flags reserved
    put_u32_le(out, h.slot_index);
    put_u32_le(out, h.width);
    put_u32_le(out, h.height);

    // fourcc: write 4 bytes from h.format, padding with '\0' if shorter.
    for (size_t i = 0; i < 4; ++i) {
        out.push_back(i < h.format.size() ? static_cast<uint8_t>(h.format[i]) : 0);
    }

    put_u64_le(out, h.frame_idx);
    out.push_back(static_cast<uint8_t>(h.color_matrix));
    out.push_back(static_cast<uint8_t>(h.color_range));
    out.push_back(static_cast<uint8_t>(h.chroma_siting));
    out.push_back(static_cast<uint8_t>(plane_count));

    for (uint32_t p : h.plane_pitches) {
        put_u32_le(out, p);
    }
    for (uint32_t p : h.plane_offsets) {
        put_u32_le(out, p);
    }
    return out;
}

bool Decode(std::span<const uint8_t> bytes, Header& out, std::string* err) {
    if (bytes.size() < 36) {
        return set_err(err, "header bytes < 36");
    }
    const uint8_t* p = bytes.data();
    const uint32_t magic = get_u32_le(p);
    if (magic != kMagic) {
        return set_err(err, "bad magic");
    }
    const uint16_t version = get_u16_le(p + 4);
    if (version != kVersion) {
        return set_err(err, "unsupported version");
    }
    // flags at p+6: reserved, must be 0 today. Unknown flag bits are
    // tolerated in case a future minor version adds opt-in semantics.
    out.slot_index = get_u32_le(p + 8);
    out.width = get_u32_le(p + 12);
    out.height = get_u32_le(p + 16);
    out.format.assign(reinterpret_cast<const char*>(p + 20), 4);
    // Trim trailing nuls so callers comparing to "NV12" (3-char fourccs
    // pad with \0) don't see a longer string than they expect.
    while (!out.format.empty() && out.format.back() == '\0') {
        out.format.pop_back();
    }
    out.frame_idx = get_u64_le(p + 24);
    out.color_matrix = static_cast<ColorMatrix>(p[32]);
    out.color_range = static_cast<ColorRange>(p[33]);
    out.chroma_siting = static_cast<ChromaSiting>(p[34]);
    const uint8_t plane_count = p[35];
    if (plane_count == 0 || plane_count > kMaxPlanes) {
        return set_err(err, "plane_count out of range");
    }
    const size_t expected = SerializedSize(plane_count);
    if (bytes.size() < expected) {
        return set_err(err, "truncated plane arrays");
    }
    out.plane_pitches.resize(plane_count);
    out.plane_offsets.resize(plane_count);
    const uint8_t* pitches = p + 36;
    const uint8_t* offsets = pitches + 4 * plane_count;
    for (size_t i = 0; i < plane_count; ++i) {
        out.plane_pitches[i] = get_u32_le(pitches + 4 * i);
        out.plane_offsets[i] = get_u32_le(offsets + 4 * i);
    }
    return true;
}

} // namespace dmabuf_header
