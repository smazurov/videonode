#include "dmabuf_msg.hpp"

#include <cctype>
#include <cstdio>
#include <cstdlib>
#include <sstream>

namespace dmabuf_msg {

namespace {

// Skip JSON whitespace at p; return new position.
size_t skip_ws(std::string_view s, size_t p) {
    while (p < s.size() && (s[p] == ' ' || s[p] == '\t' || s[p] == '\n' || s[p] == '\r'))
        ++p;
    return p;
}

// Parse a JSON string starting at s[p] (which should be '"'). On success
// returns the next position after the closing quote and writes the
// decoded value to `out`. Returns std::string::npos on malformed input.
size_t parse_string(std::string_view s, size_t p, std::string& out) {
    if (p >= s.size() || s[p] != '"')
        return std::string::npos;
    ++p;
    out.clear();
    while (p < s.size() && s[p] != '"') {
        if (s[p] == '\\' && p + 1 < s.size()) {
            // Handle the JSON escapes we'd ever see from Go's encoding/json.
            char c = s[p + 1];
            switch (c) {
            case '"':
                out += '"';
                break;
            case '\\':
                out += '\\';
                break;
            case '/':
                out += '/';
                break;
            case 'n':
                out += '\n';
                break;
            case 't':
                out += '\t';
                break;
            case 'r':
                out += '\r';
                break;
            default:
                // We don't expect \u escapes in our specific schema.
                out += c;
                break;
            }
            p += 2;
        } else {
            out += s[p++];
        }
    }
    if (p >= s.size())
        return std::string::npos;
    return p + 1; // skip closing "
}

// Parse a JSON unsigned integer starting at s[p]. Returns npos on
// malformed input.
size_t parse_uint(std::string_view s, size_t p, uint64_t& out) {
    if (p >= s.size() || !std::isdigit(static_cast<unsigned char>(s[p]))) {
        return std::string::npos;
    }
    out = 0;
    while (p < s.size() && std::isdigit(static_cast<unsigned char>(s[p]))) {
        out = out * 10 + static_cast<uint64_t>(s[p] - '0');
        ++p;
    }
    return p;
}

// Parse a JSON array of unsigned integers ("[1, 2, 3]") into `out`.
size_t parse_uint_array(std::string_view s, size_t p, std::vector<uint32_t>& out) {
    p = skip_ws(s, p);
    if (p >= s.size() || s[p] != '[')
        return std::string::npos;
    ++p;
    out.clear();
    p = skip_ws(s, p);
    if (p < s.size() && s[p] == ']')
        return p + 1; // empty array
    while (p < s.size()) {
        p = skip_ws(s, p);
        uint64_t v = 0;
        p = parse_uint(s, p, v);
        if (p == std::string::npos)
            return std::string::npos;
        out.push_back(static_cast<uint32_t>(v));
        p = skip_ws(s, p);
        if (p >= s.size())
            return std::string::npos;
        if (s[p] == ',') {
            ++p;
            continue;
        }
        if (s[p] == ']')
            return p + 1;
        return std::string::npos;
    }
    return std::string::npos;
}

void set_err(std::string* err, const char* msg) {
    if (err)
        *err = msg;
}

} // namespace

bool DecodeHeader(std::string_view s, Header& out, std::string* err) {
    out = Header{};
    size_t p = skip_ws(s, 0);
    if (p >= s.size() || s[p] != '{') {
        set_err(err, "expected '{'");
        return false;
    }
    ++p;

    while (true) {
        p = skip_ws(s, p);
        if (p >= s.size()) {
            set_err(err, "unexpected end inside object");
            return false;
        }
        if (s[p] == '}') {
            ++p;
            break;
        }

        std::string key;
        p = parse_string(s, p, key);
        if (p == std::string::npos) {
            set_err(err, "bad key");
            return false;
        }
        p = skip_ws(s, p);
        if (p >= s.size() || s[p] != ':') {
            set_err(err, "expected ':'");
            return false;
        }
        ++p;
        p = skip_ws(s, p);

        if (key == "slot_index" || key == "width" || key == "height" || key == "frame_idx") {
            uint64_t v = 0;
            size_t np = parse_uint(s, p, v);
            if (np == std::string::npos) {
                set_err(err, "bad number for key");
                return false;
            }
            if (key == "slot_index")
                out.slot_index = static_cast<uint32_t>(v);
            else if (key == "width")
                out.width = static_cast<uint32_t>(v);
            else if (key == "height")
                out.height = static_cast<uint32_t>(v);
            else
                out.frame_idx = v;
            p = np;
        } else if (key == "format") {
            std::string v;
            size_t np = parse_string(s, p, v);
            if (np == std::string::npos) {
                set_err(err, "bad string for format");
                return false;
            }
            out.format = std::move(v);
            p = np;
        } else if (key == "plane_pitches") {
            size_t np = parse_uint_array(s, p, out.plane_pitches);
            if (np == std::string::npos) {
                set_err(err, "bad plane_pitches array");
                return false;
            }
            p = np;
        } else if (key == "plane_offsets") {
            size_t np = parse_uint_array(s, p, out.plane_offsets);
            if (np == std::string::npos) {
                set_err(err, "bad plane_offsets array");
                return false;
            }
            p = np;
        } else {
            // Forward-compat: ignore unknown keys, but skip their value
            // safely. Supported value types: number, string, array, true/false/null.
            // For simplicity we just scan to the next comma or closing brace at
            // brace/bracket depth 0.
            int depth = 0;
            bool in_string = false;
            while (p < s.size()) {
                char c = s[p];
                if (in_string) {
                    if (c == '\\') {
                        p += 2;
                        continue;
                    }
                    if (c == '"') {
                        in_string = false;
                        ++p;
                        continue;
                    }
                    ++p;
                    continue;
                }
                if (c == '"') {
                    in_string = true;
                    ++p;
                    continue;
                }
                if (c == '[' || c == '{') {
                    ++depth;
                    ++p;
                    continue;
                }
                if (c == ']' || c == '}') {
                    if (depth == 0)
                        break;
                    --depth;
                    ++p;
                    continue;
                }
                if (c == ',' && depth == 0)
                    break;
                ++p;
            }
        }

        p = skip_ws(s, p);
        if (p >= s.size()) {
            set_err(err, "unexpected end after value");
            return false;
        }
        if (s[p] == ',') {
            ++p;
            continue;
        }
        if (s[p] == '}') {
            ++p;
            break;
        }
        set_err(err, "expected ',' or '}'");
        return false;
    }

    // Sanity: pitches and offsets must agree in length.
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

std::string EncodeHeader(const Header& h) {
    std::ostringstream o;
    o << "{";
    o << R"("slot_index":)" << h.slot_index;
    o << R"(,"width":)" << h.width;
    o << R"(,"height":)" << h.height;
    o << R"(,"format":")" << h.format << "\"";
    o << R"(,"plane_pitches":[)";
    for (size_t i = 0; i < h.plane_pitches.size(); ++i) {
        if (i)
            o << ",";
        o << h.plane_pitches[i];
    }
    o << "]";
    o << R"(,"plane_offsets":[)";
    for (size_t i = 0; i < h.plane_offsets.size(); ++i) {
        if (i)
            o << ",";
        o << h.plane_offsets[i];
    }
    o << "]";
    o << R"(,"frame_idx":)" << h.frame_idx;
    o << "}";
    return o.str();
}

} // namespace dmabuf_msg
