// dmabuf_msg — wire format for dma-buf fd handoff from the Go daemon to
// composer-spike over a Unix socket with SCM_RIGHTS ancillary data.
//
// Layout (mirror of internal/composer/dmabuf.go):
//
//   [4 bytes big-endian: length of JSON header] [JSON bytes...]
//   + ancillary data carrying SCM_RIGHTS file descriptors
//
// JSON shape:
//
//   {
//     "slot_index": 0,
//     "width": 1920,
//     "height": 1080,
//     "format": "NV12",
//     "plane_pitches": [1920],
//     "plane_offsets": [0],
//     "frame_idx": 42
//   }
//
// The plane arrays have length == number of fds in the ancillary data.
// One fd = chroma packed in the same dma-buf (typical NV12 from
// rk_hdmirx or UVC); two fds = NV12M split-chroma.
//
// We hand-roll the JSON decoder because (a) we control the producer so
// the format is fixed, (b) every C++ JSON dep we'd reach for adds
// thousands of header lines we don't want compiling per .cpp file. The
// decoder is ~80 LOC and unit-tested.

#pragma once

#include <cstdint>
#include <string>
#include <string_view>
#include <vector>

namespace dmabuf_msg {

struct Header {
    uint32_t slot_index = 0;
    uint32_t width = 0;
    uint32_t height = 0;
    std::string format; // DRM fourcc (e.g. "NV12")
    std::vector<uint32_t> plane_pitches;
    std::vector<uint32_t> plane_offsets;
    uint64_t frame_idx = 0;
};

// DecodeHeader parses a JSON header from `json_bytes` and populates `out`.
// Returns true on success. On failure `err` (if non-null) is set to a
// short diagnostic string.
//
// The decoder is strict: it rejects unexpected keys with a warning logged
// to stderr (but still continues — forward-compat in case the daemon
// gains a field we don't care about), and rejects malformed JSON
// outright.
bool DecodeHeader(std::string_view json_bytes, Header& out, std::string* err = nullptr);

// EncodeHeader produces the JSON for `h`. Used by tests + by any future
// composer-side tool that wants to drive the receiving end. Returns the
// raw bytes without a length prefix.
std::string EncodeHeader(const Header& h);

} // namespace dmabuf_msg
