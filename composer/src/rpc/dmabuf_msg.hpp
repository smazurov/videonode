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
//       "plane_pitches": [1920, 1920],
//       "plane_offsets": [0, 2073600],
//       "color_matrix": 1,
//       "color_range": 1,
//       "chroma_siting": 1,
//       "frame_idx": 42
//     }
//   }
//
// The plane arrays have length == number of fds in the ancillary data.
// One fd = chroma packed in the same dma-buf (typical NV12 from
// rk_hdmirx or UVC); two fds = NV12M split-chroma.
//
// ── CSC backend contract ──
// videonode-source has more than one CSC backend (RGA on RK3588, GLES on
// generic Mesa boxes). Consumers don't care which backend produced the
// frame, but downstream encoders + samplers need consistent color
// metadata. The producer declares what it actually emitted via the
// color_matrix / color_range / chroma_siting enums below. Both backends
// MUST converge on the same triple so that swapping producers leaves
// the rest of the pipeline unchanged. The current contract is
// `Bt601 / Limited / Mpeg2` — what librga's `IM_COLOR_SPACE_DEFAULT`
// produces today. Any new backend matches that or breaks the contract.
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

enum class ColorMatrix : uint8_t {
    Unspecified = 0, // consumer fall-back: assume Bt601 (matches RGA default)
    Bt601 = 1,
    Bt709 = 2,
    Bt2020 = 3,
};

enum class ColorRange : uint8_t {
    Unspecified = 0, // consumer fall-back: assume Limited
    Limited = 1,     // 16-235 luma / 16-240 chroma (broadcast)
    Full = 2,        // 0-255 (PC / JPEG)
};

enum class ChromaSiting : uint8_t {
    Unspecified = 0, // consumer fall-back: assume Mpeg2
    Mpeg2 = 1,       // chroma left-aligned to luma (H.264 / RGA)
    Jpeg = 2,        // chroma centered (MPEG-1 / JPEG)
};

struct Header {
    uint32_t slot_index = 0;
    uint32_t width = 0;
    uint32_t height = 0;
    std::string format; // DRM fourcc (e.g. "NV12")
    std::vector<uint32_t> plane_pitches;
    std::vector<uint32_t> plane_offsets;
    ColorMatrix color_matrix = ColorMatrix::Unspecified;
    ColorRange color_range = ColorRange::Unspecified;
    ChromaSiting chroma_siting = ChromaSiting::Unspecified;
    uint64_t frame_idx = 0;
};

// DecodeFrameNotification parses a full JSON-RPC 2.0 envelope (the bytes
// after the 4-byte length prefix) and populates `out`. The envelope MUST
// be a notification with `method == "frame"`. Returns true on success.
// On failure `err` (if non-null) is set to a short diagnostic.
[[nodiscard]] bool DecodeFrameNotification(std::string_view envelope_bytes, Header& out,
                                           std::string* err = nullptr);

// EncodeFrameNotification produces the full JSON-RPC envelope ready to be
// length-prefixed and sent. Returns the raw bytes without a length prefix.
std::string EncodeFrameNotification(const Header& h);

} // namespace dmabuf_msg
