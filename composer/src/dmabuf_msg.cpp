#include "dmabuf_msg.hpp"

#include "jsonrpc_msg.hpp"

#include <sstream>

namespace dmabuf_msg {

namespace {

void set_err(std::string* err, const char* msg) {
    if (err)
        *err = msg;
}

// Parse the inner params object into a Header. Caller guarantees that
// `s` is the raw substring of `"params"` (a JSON object).
bool decode_params(std::string_view s, Header& out, std::string* err) {
    using namespace jsonrpc_msg::parse;
    out = Header{};

    size_t p = skip_ws(s, 0);
    if (p >= s.size() || s[p] != '{') {
        set_err(err, "params: expected '{'");
        return false;
    }
    ++p;

    while (true) {
        p = skip_ws(s, p);
        if (p >= s.size()) {
            set_err(err, "params: unexpected end");
            return false;
        }
        if (s[p] == '}') {
            ++p;
            break;
        }

        std::string key;
        size_t np = parse_string(s, p, key);
        if (np == std::string::npos) {
            set_err(err, "params: bad key");
            return false;
        }
        p = np;
        p = skip_ws(s, p);
        if (p >= s.size() || s[p] != ':') {
            set_err(err, "params: expected ':'");
            return false;
        }
        ++p;
        p = skip_ws(s, p);

        if (key == "slot_index" || key == "width" || key == "height" || key == "frame_idx" ||
            key == "color_matrix" || key == "color_range" || key == "chroma_siting") {
            uint64_t v = 0;
            np = parse_uint(s, p, v);
            if (np == std::string::npos) {
                set_err(err, "params: bad number for key");
                return false;
            }
            if (key == "slot_index")
                out.slot_index = static_cast<uint32_t>(v);
            else if (key == "width")
                out.width = static_cast<uint32_t>(v);
            else if (key == "height")
                out.height = static_cast<uint32_t>(v);
            else if (key == "color_matrix")
                out.color_matrix = static_cast<ColorMatrix>(v);
            else if (key == "color_range")
                out.color_range = static_cast<ColorRange>(v);
            else if (key == "chroma_siting")
                out.chroma_siting = static_cast<ChromaSiting>(v);
            else
                out.frame_idx = v;
            p = np;
        } else if (key == "format") {
            np = parse_string(s, p, out.format);
            if (np == std::string::npos) {
                set_err(err, "params: bad format");
                return false;
            }
            p = np;
        } else if (key == "plane_pitches") {
            np = parse_uint_array(s, p, out.plane_pitches);
            if (np == std::string::npos) {
                set_err(err, "params: bad plane_pitches");
                return false;
            }
            p = np;
        } else if (key == "plane_offsets") {
            np = parse_uint_array(s, p, out.plane_offsets);
            if (np == std::string::npos) {
                set_err(err, "params: bad plane_offsets");
                return false;
            }
            p = np;
        } else {
            np = skip_value(s, p);
            if (np == std::string::npos) {
                set_err(err, "params: bad value");
                return false;
            }
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
        set_err(err, "params: expected ',' or '}'");
        return false;
    }

    if (out.plane_pitches.size() != out.plane_offsets.size()) {
        set_err(err, "plane_pitches and plane_offsets length mismatch");
        return false;
    }
    if (out.plane_pitches.empty()) {
        set_err(err, "at least one plane is required");
        return false;
    }
    return true;
}

} // namespace

bool DecodeFrameNotification(std::string_view envelope_bytes, Header& out, std::string* err) {
    jsonrpc_msg::Frame frame;
    if (!jsonrpc_msg::DecodeFrame(envelope_bytes, frame, err))
        return false;
    if (frame.kind != jsonrpc_msg::FrameKind::Notification) {
        set_err(err, "expected JSON-RPC notification");
        return false;
    }
    if (frame.method != "frame") {
        set_err(err, "expected method == \"frame\"");
        return false;
    }
    if (frame.params_json.empty()) {
        set_err(err, "missing params");
        return false;
    }
    return decode_params(frame.params_json, out, err);
}

std::string EncodeFrameNotification(const Header& h) {
    std::ostringstream params;
    params << "{";
    params << R"("slot_index":)" << h.slot_index;
    params << R"(,"width":)" << h.width;
    params << R"(,"height":)" << h.height;
    params << R"(,"format":")" << h.format << "\"";
    params << R"(,"plane_pitches":[)";
    for (size_t i = 0; i < h.plane_pitches.size(); ++i) {
        if (i)
            params << ",";
        params << h.plane_pitches[i];
    }
    params << "]";
    params << R"(,"plane_offsets":[)";
    for (size_t i = 0; i < h.plane_offsets.size(); ++i) {
        if (i)
            params << ",";
        params << h.plane_offsets[i];
    }
    params << "]";
    params << R"(,"color_matrix":)" << static_cast<unsigned>(h.color_matrix);
    params << R"(,"color_range":)" << static_cast<unsigned>(h.color_range);
    params << R"(,"chroma_siting":)" << static_cast<unsigned>(h.chroma_siting);
    params << R"(,"frame_idx":)" << h.frame_idx;
    params << "}";
    return jsonrpc_msg::EncodeNotification("frame", params.str());
}

} // namespace dmabuf_msg
