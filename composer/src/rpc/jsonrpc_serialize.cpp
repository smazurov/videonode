#include "src/rpc/jsonrpc_msg.hpp"

#include <cstdio>
#include <sstream>

namespace jsonrpc_msg {

namespace {

// json_escape produces a JSON string literal (with surrounding quotes) for
// arbitrary UTF-8 input. Only the escapes we ever produce are emitted.
std::string json_escape(const std::string& s) {
    std::string o;
    o.reserve(s.size() + 2);
    o += '"';
    for (char c : s) {
        switch (c) {
        case '"':
            o += "\\\"";
            break;
        case '\\':
            o += "\\\\";
            break;
        case '\n':
            o += "\\n";
            break;
        case '\t':
            o += "\\t";
            break;
        case '\r':
            o += "\\r";
            break;
        default:
            if (static_cast<unsigned char>(c) < 0x20) {
                // Other control char: emit \u00XX
                char buf[8];
                std::snprintf(buf, sizeof(buf), "\\u%04x", static_cast<unsigned>(c));
                o += buf;
            } else {
                o += c;
            }
            break;
        }
    }
    o += '"';
    return o;
}

void append_params(std::ostringstream& o, std::string_view params_json) {
    if (params_json.empty())
        return;
    o << R"(,"params":)" << params_json;
}

} // namespace

std::string EncodeRequest(const std::string& method, std::string_view params_json,
                          std::string_view id_raw) {
    std::ostringstream o;
    o << R"({"jsonrpc":"2.0","method":)" << json_escape(method);
    append_params(o, params_json);
    o << R"(,"id":)" << id_raw << "}";
    return o.str();
}

std::string EncodeNotification(const std::string& method, std::string_view params_json) {
    std::ostringstream o;
    o << R"({"jsonrpc":"2.0","method":)" << json_escape(method);
    append_params(o, params_json);
    o << "}";
    return o.str();
}

std::string EncodeResponseResult(std::string_view result_json, std::string_view id_raw) {
    std::ostringstream o;
    o << R"({"jsonrpc":"2.0","result":)"
      << (result_json.empty() ? std::string_view("{}") : result_json) << R"(,"id":)" << id_raw
      << "}";
    return o.str();
}

std::string EncodeResponseError(int64_t code, const std::string& message,
                                std::string_view data_json, std::string_view id_raw) {
    std::ostringstream o;
    o << R"({"jsonrpc":"2.0","error":{"code":)" << code << R"(,"message":)" << json_escape(message);
    if (!data_json.empty())
        o << R"(,"data":)" << data_json;
    o << R"(},"id":)" << id_raw << "}";
    return o.str();
}

} // namespace jsonrpc_msg
