#include "src/rpc/jsonrpc_msg.hpp"

#include <cctype>

namespace jsonrpc_msg {

namespace parse {

size_t skip_ws(std::string_view s, size_t p) {
    while (p < s.size() && (s[p] == ' ' || s[p] == '\t' || s[p] == '\n' || s[p] == '\r'))
        ++p;
    return p;
}

static int hex_nibble(char c) {
    if (c >= '0' && c <= '9')
        return c - '0';
    if (c >= 'a' && c <= 'f')
        return 10 + (c - 'a');
    if (c >= 'A' && c <= 'F')
        return 10 + (c - 'A');
    return -1;
}

static void append_utf8(std::string& out, unsigned cp) {
    if (cp <= 0x7f) {
        out += static_cast<char>(cp);
    } else if (cp <= 0x7ff) {
        out += static_cast<char>(0xc0 | (cp >> 6));
        out += static_cast<char>(0x80 | (cp & 0x3f));
    } else if (cp <= 0xffff) {
        out += static_cast<char>(0xe0 | (cp >> 12));
        out += static_cast<char>(0x80 | ((cp >> 6) & 0x3f));
        out += static_cast<char>(0x80 | (cp & 0x3f));
    } else {
        out += static_cast<char>(0xf0 | (cp >> 18));
        out += static_cast<char>(0x80 | ((cp >> 12) & 0x3f));
        out += static_cast<char>(0x80 | ((cp >> 6) & 0x3f));
        out += static_cast<char>(0x80 | (cp & 0x3f));
    }
}

size_t parse_string(std::string_view s, size_t p, std::string& out) {
    if (p >= s.size() || s[p] != '"')
        return std::string::npos;
    ++p;
    out.clear();
    while (p < s.size() && s[p] != '"') {
        if (s[p] == '\\' && p + 1 < s.size()) {
            char c = s[p + 1];
            switch (c) {
            case '"':
                out += '"';
                p += 2;
                break;
            case '\\':
                out += '\\';
                p += 2;
                break;
            case '/':
                out += '/';
                p += 2;
                break;
            case 'n':
                out += '\n';
                p += 2;
                break;
            case 't':
                out += '\t';
                p += 2;
                break;
            case 'r':
                out += '\r';
                p += 2;
                break;
            case 'b':
                out += '\b';
                p += 2;
                break;
            case 'f':
                out += '\f';
                p += 2;
                break;
            case 'u': {
                if (p + 6 > s.size())
                    return std::string::npos;
                int h0 = hex_nibble(s[p + 2]);
                int h1 = hex_nibble(s[p + 3]);
                int h2 = hex_nibble(s[p + 4]);
                int h3 = hex_nibble(s[p + 5]);
                if (h0 < 0 || h1 < 0 || h2 < 0 || h3 < 0)
                    return std::string::npos;
                unsigned cp =
                    (unsigned(h0) << 12) | (unsigned(h1) << 8) | (unsigned(h2) << 4) | unsigned(h3);
                // Surrogate-pair handling: high surrogate (D800-DBFF) must
                // be followed by a low surrogate (DC00-DFFF).
                if (cp >= 0xd800 && cp <= 0xdbff && p + 12 <= s.size() && s[p + 6] == '\\' &&
                    s[p + 7] == 'u') {
                    int g0 = hex_nibble(s[p + 8]);
                    int g1 = hex_nibble(s[p + 9]);
                    int g2 = hex_nibble(s[p + 10]);
                    int g3 = hex_nibble(s[p + 11]);
                    if (g0 < 0 || g1 < 0 || g2 < 0 || g3 < 0)
                        return std::string::npos;
                    unsigned low = (unsigned(g0) << 12) | (unsigned(g1) << 8) |
                                   (unsigned(g2) << 4) | unsigned(g3);
                    if (low >= 0xdc00 && low <= 0xdfff) {
                        cp = 0x10000 + ((cp - 0xd800) << 10) + (low - 0xdc00);
                        p += 12;
                    } else {
                        return std::string::npos;
                    }
                } else {
                    p += 6;
                }
                append_utf8(out, cp);
                break;
            }
            default:
                out += c;
                p += 2;
                break;
            }
        } else {
            out += s[p++];
        }
    }
    if (p >= s.size())
        return std::string::npos;
    return p + 1;
}

size_t parse_uint(std::string_view s, size_t p, uint64_t& out) {
    if (p >= s.size() || !std::isdigit(static_cast<unsigned char>(s[p])))
        return std::string::npos;
    out = 0;
    while (p < s.size() && std::isdigit(static_cast<unsigned char>(s[p]))) {
        out = out * 10 + static_cast<uint64_t>(s[p] - '0');
        ++p;
    }
    return p;
}

size_t parse_int(std::string_view s, size_t p, int64_t& out) {
    bool neg = false;
    if (p < s.size() && s[p] == '-') {
        neg = true;
        ++p;
    }
    uint64_t mag = 0;
    size_t np = parse_uint(s, p, mag);
    if (np == std::string::npos)
        return std::string::npos;
    out = neg ? -static_cast<int64_t>(mag) : static_cast<int64_t>(mag);
    return np;
}

size_t parse_uint_array(std::string_view s, size_t p, std::vector<uint32_t>& out) {
    p = skip_ws(s, p);
    if (p >= s.size() || s[p] != '[')
        return std::string::npos;
    ++p;
    out.clear();
    p = skip_ws(s, p);
    if (p < s.size() && s[p] == ']')
        return p + 1;
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

size_t skip_value(std::string_view s, size_t p) {
    p = skip_ws(s, p);
    if (p >= s.size())
        return std::string::npos;
    char c = s[p];
    // Object or array: depth-aware scan.
    if (c == '{' || c == '[') {
        char open = c, close = (c == '{') ? '}' : ']';
        int depth = 1;
        ++p;
        bool in_string = false;
        while (p < s.size() && depth > 0) {
            char ch = s[p];
            if (in_string) {
                if (ch == '\\' && p + 1 < s.size()) {
                    p += 2;
                    continue;
                }
                if (ch == '"')
                    in_string = false;
                ++p;
                continue;
            }
            if (ch == '"') {
                in_string = true;
                ++p;
                continue;
            }
            if (ch == open) {
                ++depth;
                ++p;
                continue;
            }
            if (ch == close) {
                --depth;
                ++p;
                continue;
            }
            ++p;
        }
        return depth == 0 ? p : std::string::npos;
    }
    // String.
    if (c == '"') {
        std::string tmp;
        return parse_string(s, p, tmp);
    }
    // Literal: true / false / null.
    if (c == 't' || c == 'f' || c == 'n') {
        size_t end = p;
        while (end < s.size() && std::isalpha(static_cast<unsigned char>(s[end])))
            ++end;
        return end;
    }
    // Number.
    if (c == '-' || std::isdigit(static_cast<unsigned char>(c))) {
        size_t end = p;
        if (s[end] == '-')
            ++end;
        while (end < s.size() &&
               (std::isdigit(static_cast<unsigned char>(s[end])) || s[end] == '.' ||
                s[end] == 'e' || s[end] == 'E' || s[end] == '+' || s[end] == '-'))
            ++end;
        return end;
    }
    return std::string::npos;
}

size_t skip_unknown_value(std::string_view s, size_t p) {
    // Skip a value and stop at the surrounding ',' or '}' at depth 0.
    return skip_value(s, p);
}

} // namespace parse

namespace {

void set_err(std::string* err, const char* msg) {
    if (err)
        *err = msg;
}

} // namespace

bool DecodeFrame(std::string_view s, Frame& out, std::string* err) {
    using namespace parse;
    out = Frame{};

    size_t p = skip_ws(s, 0);
    if (p >= s.size() || s[p] != '{') {
        set_err(err, "expected '{'");
        return false;
    }
    ++p;

    bool saw_jsonrpc = false;
    bool saw_method = false;
    bool saw_params = false;
    bool saw_id = false;
    bool saw_result = false;
    bool saw_error = false;

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
        size_t np = parse_string(s, p, key);
        if (np == std::string::npos) {
            set_err(err, "bad key");
            return false;
        }
        p = np;

        p = skip_ws(s, p);
        if (p >= s.size() || s[p] != ':') {
            set_err(err, "expected ':'");
            return false;
        }
        ++p;
        p = skip_ws(s, p);

        if (key == "jsonrpc") {
            std::string v;
            np = parse_string(s, p, v);
            if (np == std::string::npos) {
                set_err(err, "bad jsonrpc string");
                return false;
            }
            if (v != "2.0") {
                set_err(err, "jsonrpc must be \"2.0\"");
                return false;
            }
            saw_jsonrpc = true;
            p = np;
        } else if (key == "method") {
            np = parse_string(s, p, out.method);
            if (np == std::string::npos) {
                set_err(err, "bad method");
                return false;
            }
            saw_method = true;
            p = np;
        } else if (key == "params") {
            // Capture as raw substring.
            size_t start = p;
            np = skip_value(s, p);
            if (np == std::string::npos) {
                set_err(err, "bad params");
                return false;
            }
            out.params_json.assign(s.substr(start, np - start));
            saw_params = true;
            p = np;
        } else if (key == "id") {
            // Capture as raw substring (covers number / string / null).
            size_t start = p;
            np = skip_value(s, p);
            if (np == std::string::npos) {
                set_err(err, "bad id");
                return false;
            }
            out.id_raw.assign(s.substr(start, np - start));
            out.has_id = true;
            saw_id = true;
            p = np;
        } else if (key == "result") {
            size_t start = p;
            np = skip_value(s, p);
            if (np == std::string::npos) {
                set_err(err, "bad result");
                return false;
            }
            out.result_json.assign(s.substr(start, np - start));
            out.has_result = true;
            saw_result = true;
            p = np;
        } else if (key == "error") {
            // Drill in: {"code":N,"message":"...","data":...}
            p = skip_ws(s, p);
            if (p >= s.size() || s[p] != '{') {
                set_err(err, "error must be object");
                return false;
            }
            ++p;
            while (true) {
                p = skip_ws(s, p);
                if (p >= s.size()) {
                    set_err(err, "unexpected end in error object");
                    return false;
                }
                if (s[p] == '}') {
                    ++p;
                    break;
                }
                std::string ekey;
                size_t en = parse_string(s, p, ekey);
                if (en == std::string::npos) {
                    set_err(err, "bad error key");
                    return false;
                }
                p = en;
                p = skip_ws(s, p);
                if (p >= s.size() || s[p] != ':') {
                    set_err(err, "expected ':' in error");
                    return false;
                }
                ++p;
                p = skip_ws(s, p);
                if (ekey == "code") {
                    int64_t code = 0;
                    en = parse_int(s, p, code);
                    if (en == std::string::npos) {
                        set_err(err, "bad error.code");
                        return false;
                    }
                    out.error_code = code;
                    p = en;
                } else if (ekey == "message") {
                    en = parse_string(s, p, out.error_message);
                    if (en == std::string::npos) {
                        set_err(err, "bad error.message");
                        return false;
                    }
                    p = en;
                } else if (ekey == "data") {
                    size_t start = p;
                    en = skip_value(s, p);
                    if (en == std::string::npos) {
                        set_err(err, "bad error.data");
                        return false;
                    }
                    out.error_data_json.assign(s.substr(start, en - start));
                    p = en;
                } else {
                    en = skip_value(s, p);
                    if (en == std::string::npos) {
                        set_err(err, "bad value in error");
                        return false;
                    }
                    p = en;
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
                set_err(err, "expected ',' or '}' in error");
                return false;
            }
            out.has_error = true;
            saw_error = true;
        } else {
            // Unknown key — skip its value for forward compat.
            np = skip_value(s, p);
            if (np == std::string::npos) {
                set_err(err, "bad value in object");
                return false;
            }
            p = np;
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

    if (!saw_jsonrpc) {
        set_err(err, "missing \"jsonrpc\":\"2.0\"");
        return false;
    }
    // Classify.
    if (saw_method && saw_id) {
        out.kind = FrameKind::Request;
    } else if (saw_method && !saw_id) {
        out.kind = FrameKind::Notification;
    } else if (saw_id && (saw_result ^ saw_error)) {
        out.kind = FrameKind::Response;
    } else {
        set_err(err, "frame does not match Request/Notification/Response shape");
        return false;
    }
    (void)saw_params;
    return true;
}

} // namespace jsonrpc_msg
