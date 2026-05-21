// dmabuf_msg — wire format for dma-buf fd handoff from a videonode-source
// producer to its consumers (videonode-sink, composer-spike, snapshot, AI,
// recording) over a Unix socket with SCM_RIGHTS ancillary data.
//
// Layout:
//
//   [4 bytes big-endian: length of JSON envelope] [JSON bytes...]
//   + ancillary data carrying SCM_RIGHTS file descriptors
//
// The envelope is a JSON-RPC 2.0 *notification* (no `id`, no reply
// expected) with method `"frame"`:
//
//   {
//     "jsonrpc": "2.0",
//     "method": "frame",
//     "params": {
//       "slot_index": 0,
//       "width": 1920,
//       "height": 1080,
//       "format": "NV12",
//       "plane_pitches": [1920],
//       "plane_offsets": [0],
//       "frame_idx": 42
//     }
//   }
//
// The plane arrays have length == number of fds in the ancillary data.
// One fd = chroma packed in the same dma-buf (typical NV12 from
// rk_hdmirx or UVC); two fds = NV12M split-chroma.
//
// The codec is layered on top of jsonrpc_msg (shared envelope parser) so
// the control plane and data plane share one mental model and one parser
// implementation.

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

// DecodeFrameNotification parses a full JSON-RPC 2.0 envelope (the bytes
// after the 4-byte length prefix) and populates `out`. The envelope MUST
// be a notification with `method == "frame"`. Returns true on success.
// On failure `err` (if non-null) is set to a short diagnostic.
bool DecodeFrameNotification(std::string_view envelope_bytes, Header& out,
                             std::string* err = nullptr);

// EncodeFrameNotification produces the full JSON-RPC envelope ready to be
// length-prefixed and sent. Returns the raw bytes without a length prefix.
std::string EncodeFrameNotification(const Header& h);

} // namespace dmabuf_msg
