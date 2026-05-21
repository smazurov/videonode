#include "src/source/set_format_parser.hpp"

#include "src/rpc/jsonrpc_msg.hpp"

namespace source {

namespace {

constexpr int kInvalidParams = -32602;

// Soft caps — anything larger is almost certainly a typo or attack.
// V4L2 + HDMI capture practically tops out below these.
constexpr uint64_t kMaxDim = 16384;
constexpr uint64_t kMaxFps = 240;

bool fail(SetFormatError& err, int code, const char* msg) {
    err.code = code;
    err.message = msg;
    return false;
}

} // namespace

bool parse_set_format(std::string_view s, SetFormatRequest& out, SetFormatError& err) {
    using namespace jsonrpc_msg::parse;

    size_t p = skip_ws(s, 0);
    if (p >= s.size() || s[p] != '{')
        return fail(err, kInvalidParams, "params must be object");
    ++p;

    std::string fourcc;
    uint64_t w = 0, h = 0, fps = 0;
    bool got_fourcc = false, got_w = false, got_h = false;

    while (true) {
        p = skip_ws(s, p);
        if (p >= s.size())
            return fail(err, kInvalidParams, "truncated params");
        if (s[p] == '}') {
            ++p;
            break;
        }

        std::string key;
        size_t np = parse_string(s, p, key);
        if (np == std::string::npos)
            return fail(err, kInvalidParams, "bad key");
        p = np;

        p = skip_ws(s, p);
        if (p >= s.size() || s[p] != ':')
            return fail(err, kInvalidParams, "expected ':'");
        ++p;
        p = skip_ws(s, p);

        if (key == "fourcc") {
            np = parse_string(s, p, fourcc);
            if (np == std::string::npos)
                return fail(err, kInvalidParams, "bad fourcc");
            got_fourcc = true;
            p = np;
        } else if (key == "w" || key == "h" || key == "fps") {
            uint64_t v = 0;
            np = parse_uint(s, p, v);
            if (np == std::string::npos)
                return fail(err, kInvalidParams, "bad numeric field");
            if (key == "w") {
                w = v;
                got_w = true;
            } else if (key == "h") {
                h = v;
                got_h = true;
            } else {
                fps = v;
            }
            p = np;
        } else {
            // Unknown keys are silently skipped for forward compatibility.
            np = skip_value(s, p);
            if (np == std::string::npos)
                return fail(err, kInvalidParams, "bad value");
            p = np;
        }

        p = skip_ws(s, p);
        if (p < s.size() && s[p] == ',') {
            ++p;
            continue;
        }
        if (p < s.size() && s[p] == '}') {
            ++p;
            break;
        }
        return fail(err, kInvalidParams, "expected ',' or '}'");
    }

    if (!got_fourcc || !got_w || !got_h)
        return fail(err, kInvalidParams, "missing required field (fourcc, w, h)");
    if (w == 0 || h == 0)
        return fail(err, kInvalidParams, "w and h must be > 0");
    if (w > kMaxDim || h > kMaxDim)
        return fail(err, kInvalidParams, "w / h exceed sane upper bound (16384)");
    if (w % 2 != 0 || h % 2 != 0)
        return fail(err, kInvalidParams, "w and h must be even (NV12 requires aligned chroma)");
    if (fps > kMaxFps)
        return fail(err, kInvalidParams, "fps exceeds sane upper bound (240)");

    out.fourcc = std::move(fourcc);
    out.w = uint32_t(w);
    out.h = uint32_t(h);
    out.fps = uint32_t(fps);
    return true;
}

} // namespace source
