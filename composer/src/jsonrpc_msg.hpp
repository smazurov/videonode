// jsonrpc_msg — JSON-RPC 2.0 envelope codec shared by both the data plane
// (videonode-source ↔ videonode-sink dma-buf frames, framed with a 4-byte
// big-endian length prefix) and the control plane (videonode-source ↔ Go
// daemon, framed with newlines).
//
// Spec: https://www.jsonrpc.org/specification
//
// We hand-roll the codec for the same reasons the original dmabuf_msg did:
// the project already avoids C++ JSON dependencies, the parser is small
// (~250 LOC), and we own both producer and consumer.
//
// The codec preserves raw substrings for `params`, `result`, and
// `error.data` so callers can do method-specific parsing without a second
// pass through a generic JSON DOM.

#pragma once

#include <cstdint>
#include <string>
#include <string_view>
#include <vector>

namespace jsonrpc_msg {

// FrameKind discriminates the JSON-RPC 2.0 message variants.
//
//   Request      — has "method" and "id".  Peer expects a Response.
//   Notification — has "method" and no "id".  No Response.
//   Response     — has "id" and ("result" XOR "error").  Reply to a Request.
//   Unknown      — failed to classify (treat as protocol error).
enum class FrameKind { Unknown, Request, Notification, Response };

// Frame is the decoded envelope. Substring fields point at raw JSON values
// (object/array/string/number/...) so callers can do further decoding.
struct Frame {
    FrameKind kind = FrameKind::Unknown;

    // Request + Notification.
    std::string method;
    std::string params_json; // raw JSON, "" if absent

    // Request + Response: id is echoed verbatim (e.g. "42" or "\"abc\"")
    // so we never lose type fidelity.
    std::string id_raw;
    bool has_id = false;

    // Response success.
    std::string result_json;
    bool has_result = false;

    // Response error.
    bool has_error = false;
    int64_t error_code = 0;
    std::string error_message;
    std::string error_data_json; // optional
};

// DecodeFrame parses one JSON object into `out`. Returns true on success.
// `err` (if non-null) is set to a short diagnostic on failure.
bool DecodeFrame(std::string_view bytes, Frame& out, std::string* err = nullptr);

// Encoders. `params_json` / `result_json` / `data_json` must be a valid
// JSON value when non-empty (the encoder injects them verbatim, no
// re-escaping). `id_raw` likewise — pass `"42"` for a numeric id or
// `"\"abc\""` for a string id.
std::string EncodeRequest(const std::string& method, std::string_view params_json,
                          std::string_view id_raw);
std::string EncodeNotification(const std::string& method, std::string_view params_json);
std::string EncodeResponseResult(std::string_view result_json, std::string_view id_raw);
std::string EncodeResponseError(int64_t code, const std::string& message,
                                std::string_view data_json, std::string_view id_raw);

// Low-level helpers used by both this codec and dmabuf_msg.cpp.
namespace parse {

// Advance past JSON whitespace.
size_t skip_ws(std::string_view s, size_t p);

// Parse a JSON string starting at s[p] (must point at '"'). Writes the
// decoded value to `out`. Returns the position just past the closing
// quote, or std::string::npos on error.
size_t parse_string(std::string_view s, size_t p, std::string& out);

// Parse an unsigned integer. Returns position just past the last digit,
// or std::string::npos on error.
size_t parse_uint(std::string_view s, size_t p, uint64_t& out);

// Parse a signed integer (handles optional leading '-').
size_t parse_int(std::string_view s, size_t p, int64_t& out);

// Parse a JSON array of unsigned integers: "[1, 2, 3]". Returns position
// just past the closing ']'.
size_t parse_uint_array(std::string_view s, size_t p, std::vector<uint32_t>& out);

// Skip past any JSON value (object / array / string / number / true /
// false / null) starting at s[p]. Returns the position just past the
// value's end. Used to grab raw substrings for nested objects.
size_t skip_value(std::string_view s, size_t p);

// Skip past one key:value pair's value, stopping at the next top-level
// ',' or '}' (depth-aware). Used by forward-compat unknown-key handling.
size_t skip_unknown_value(std::string_view s, size_t p);

} // namespace parse

} // namespace jsonrpc_msg
