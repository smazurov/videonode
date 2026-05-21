// set_format_parser — JSON params parser for the `set_format` control-plane
// method, factored out of orchestrator.cpp so the wire format has its own
// unit-testable surface (untrusted bytes, reached from the Go daemon over
// a Unix socket — see rpc/control_channel and the JSON-RPC envelope codec
// in rpc/jsonrpc_msg).
//
// Expected params shape:
//   {"fourcc":"YUYV","w":1920,"h":1080,"fps":30}
//
// fourcc / w / h are required; fps is optional (0 = caller's default).
// Returns a parsed request on success; otherwise populates a JSON-RPC error
// with the appropriate code (-32602 invalid params, -32000 unsupported
// fourcc / out-of-range dimension).
#pragma once

#include <cstdint>
#include <string>
#include <string_view>

namespace source {

struct SetFormatRequest {
    std::string fourcc;
    uint32_t w = 0;
    uint32_t h = 0;
    uint32_t fps = 0; // 0 = unspecified (caller falls back to its default)
};

struct SetFormatError {
    int code = 0; // JSON-RPC error code
    std::string message;
};

// parse_set_format consumes a JSON object string and either populates
// `out` (returns true) or `err` (returns false). On error, `out` is
// left in an unspecified state — callers must not read it.
//
// Validation:
//   - top-level must be a JSON object (-32602)
//   - fourcc / w / h required, fps optional (-32602)
//   - integer fields must be > 0 and fit in uint32_t (-32602)
//   - w / h must be even and <= 16384 (-32602; rejects nonsense and the
//     casts in the caller from uint64 → int)
//   - fps if present must be <= 240 (-32602)
//
// Does NOT validate fourcc semantics — that's the caller's job (look it
// up against v4l2_pix_fmt_ in source/capture_session.hpp and return
// -32000 "unsupported fourcc" on miss). Kept out of the parser to avoid
// a source/ → capture/ dep cycle.
[[nodiscard]] bool parse_set_format(std::string_view params_json,
                                    SetFormatRequest& out,
                                    SetFormatError& err);

} // namespace source
