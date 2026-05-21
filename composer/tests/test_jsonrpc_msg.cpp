#include "../src/jsonrpc_msg.hpp"
#include "test_runner.hpp"

#include <climits>
#include <cstdint>
#include <string>

using namespace jsonrpc_msg;
using parse::parse_int;
using parse::parse_string;
using parse::parse_uint;
using parse::parse_uint_array;
using parse::skip_value;
using parse::skip_ws;

// =====================================================================
// parse helpers — exhaustive coverage.
// =====================================================================

static void test_skip_ws() {
    test_runner::start_case("skip_ws_empty");
    { CHECK_EQ(skip_ws("", 0), size_t(0)); }

    test_runner::start_case("skip_ws_no_whitespace");
    { CHECK_EQ(skip_ws("abc", 0), size_t(0)); }

    test_runner::start_case("skip_ws_spaces");
    { CHECK_EQ(skip_ws("   a", 0), size_t(3)); }

    test_runner::start_case("skip_ws_tabs_newlines_returns");
    { CHECK_EQ(skip_ws("\t\n\r a", 0), size_t(4)); }

    test_runner::start_case("skip_ws_from_middle");
    {
        // " ab   cd" — start at index 3, should skip to 'c' at 6.
        CHECK_EQ(skip_ws(" ab   cd", 3), size_t(6));
    }

    test_runner::start_case("skip_ws_only_whitespace");
    { CHECK_EQ(skip_ws("    ", 0), size_t(4)); }
}

static void test_parse_string() {
    test_runner::start_case("parse_string_empty");
    {
        std::string out;
        CHECK_EQ(parse_string("\"\"", 0, out), size_t(2));
        CHECK_STR_EQ(out, "");
    }

    test_runner::start_case("parse_string_basic");
    {
        std::string out;
        CHECK_EQ(parse_string("\"hello\"", 0, out), size_t(7));
        CHECK_STR_EQ(out, "hello");
    }

    test_runner::start_case("parse_string_escape_quote");
    {
        std::string out;
        const char* s = R"("a\"b")";
        CHECK_EQ(parse_string(s, 0, out), std::string(s).size());
        CHECK_STR_EQ(out, "a\"b");
    }

    test_runner::start_case("parse_string_escape_backslash");
    {
        std::string out;
        const char* s = R"("a\\b")";
        CHECK_EQ(parse_string(s, 0, out), std::string(s).size());
        CHECK_STR_EQ(out, "a\\b");
    }

    test_runner::start_case("parse_string_escape_slash");
    {
        std::string out;
        const char* s = R"("a\/b")";
        CHECK_EQ(parse_string(s, 0, out), std::string(s).size());
        CHECK_STR_EQ(out, "a/b");
    }

    test_runner::start_case("parse_string_escape_n_t_r");
    {
        std::string out;
        const char* s = R"("a\nb\tc\rd")";
        CHECK_EQ(parse_string(s, 0, out), std::string(s).size());
        CHECK_STR_EQ(out, "a\nb\tc\rd");
    }

    test_runner::start_case("parse_string_rejects_missing_open_quote");
    {
        std::string out;
        CHECK_EQ(parse_string("abc", 0, out), std::string::npos);
    }

    test_runner::start_case("parse_string_rejects_unterminated");
    {
        std::string out;
        CHECK_EQ(parse_string("\"abc", 0, out), std::string::npos);
    }

    test_runner::start_case("parse_string_rejects_eof_after_open_quote");
    {
        std::string out;
        CHECK_EQ(parse_string("\"", 0, out), std::string::npos);
    }
}

static void test_parse_uint() {
    test_runner::start_case("parse_uint_zero");
    {
        uint64_t v = 99;
        CHECK_EQ(parse_uint("0", 0, v), size_t(1));
        CHECK_EQ(v, uint64_t(0));
    }

    test_runner::start_case("parse_uint_basic");
    {
        uint64_t v = 0;
        CHECK_EQ(parse_uint("12345", 0, v), size_t(5));
        CHECK_EQ(v, uint64_t(12345));
    }

    test_runner::start_case("parse_uint_large");
    {
        uint64_t v = 0;
        CHECK_EQ(parse_uint("18446744073709551615", 0, v), size_t(20));
        CHECK_EQ(v, UINT64_MAX);
    }

    test_runner::start_case("parse_uint_stops_at_nondigit");
    {
        uint64_t v = 0;
        CHECK_EQ(parse_uint("42abc", 0, v), size_t(2));
        CHECK_EQ(v, uint64_t(42));
    }

    test_runner::start_case("parse_uint_rejects_negative");
    {
        uint64_t v = 0;
        CHECK_EQ(parse_uint("-1", 0, v), std::string::npos);
    }

    test_runner::start_case("parse_uint_rejects_non_digit");
    {
        uint64_t v = 0;
        CHECK_EQ(parse_uint("abc", 0, v), std::string::npos);
    }

    test_runner::start_case("parse_uint_rejects_empty");
    {
        uint64_t v = 0;
        CHECK_EQ(parse_uint("", 0, v), std::string::npos);
    }
}

static void test_parse_int() {
    test_runner::start_case("parse_int_positive");
    {
        int64_t v = 0;
        CHECK_EQ(parse_int("42", 0, v), size_t(2));
        CHECK_EQ(v, int64_t(42));
    }

    test_runner::start_case("parse_int_negative");
    {
        int64_t v = 0;
        CHECK_EQ(parse_int("-42", 0, v), size_t(3));
        CHECK_EQ(v, int64_t(-42));
    }

    test_runner::start_case("parse_int_negative_jsonrpc_error_code");
    {
        int64_t v = 0;
        CHECK_EQ(parse_int("-32700", 0, v), size_t(6));
        CHECK_EQ(v, int64_t(-32700));
    }

    test_runner::start_case("parse_int_zero");
    {
        int64_t v = 1;
        CHECK_EQ(parse_int("0", 0, v), size_t(1));
        CHECK_EQ(v, int64_t(0));
    }

    test_runner::start_case("parse_int_rejects_solo_minus");
    {
        int64_t v = 0;
        CHECK_EQ(parse_int("-", 0, v), std::string::npos);
    }
}

static void test_parse_uint_array() {
    test_runner::start_case("parse_uint_array_empty");
    {
        std::vector<uint32_t> a{99};
        CHECK_EQ(parse_uint_array("[]", 0, a), size_t(2));
        CHECK_EQ(a.size(), size_t(0));
    }

    test_runner::start_case("parse_uint_array_single");
    {
        std::vector<uint32_t> a;
        CHECK_EQ(parse_uint_array("[42]", 0, a), size_t(4));
        CHECK_EQ(a.size(), size_t(1));
        CHECK_EQ(a[0], uint32_t(42));
    }

    test_runner::start_case("parse_uint_array_multi");
    {
        std::vector<uint32_t> a;
        CHECK_EQ(parse_uint_array("[1,2,3]", 0, a), size_t(7));
        CHECK_EQ(a.size(), size_t(3));
        CHECK_EQ(a[0], uint32_t(1));
        CHECK_EQ(a[1], uint32_t(2));
        CHECK_EQ(a[2], uint32_t(3));
    }

    test_runner::start_case("parse_uint_array_spaces");
    {
        std::vector<uint32_t> a;
        CHECK_EQ(parse_uint_array("[ 1 , 2 , 3 ]", 0, a), size_t(13));
        CHECK_EQ(a.size(), size_t(3));
    }

    test_runner::start_case("parse_uint_array_rejects_missing_bracket");
    {
        std::vector<uint32_t> a;
        CHECK_EQ(parse_uint_array("1,2,3", 0, a), std::string::npos);
    }

    test_runner::start_case("parse_uint_array_rejects_unterminated");
    {
        std::vector<uint32_t> a;
        CHECK_EQ(parse_uint_array("[1,2", 0, a), std::string::npos);
    }

    test_runner::start_case("parse_uint_array_rejects_trailing_comma");
    {
        std::vector<uint32_t> a;
        CHECK_EQ(parse_uint_array("[1,]", 0, a), std::string::npos);
    }
}

static void test_skip_value() {
    test_runner::start_case("skip_value_string");
    { CHECK_EQ(skip_value(R"("abc")", 0), size_t(5)); }

    test_runner::start_case("skip_value_string_with_escape");
    {
        // R"("a\"b")" is 6 characters: " a \ " b "
        CHECK_EQ(skip_value(R"("a\"b")", 0), size_t(6));
    }

    test_runner::start_case("skip_value_uint");
    { CHECK_EQ(skip_value("42", 0), size_t(2)); }

    test_runner::start_case("skip_value_int_negative");
    { CHECK_EQ(skip_value("-42", 0), size_t(3)); }

    test_runner::start_case("skip_value_float");
    { CHECK_EQ(skip_value("3.14", 0), size_t(4)); }

    test_runner::start_case("skip_value_exponent");
    { CHECK_EQ(skip_value("1e10", 0), size_t(4)); }

    test_runner::start_case("skip_value_neg_exponent");
    { CHECK_EQ(skip_value("1.5e-3", 0), size_t(6)); }

    test_runner::start_case("skip_value_true");
    { CHECK_EQ(skip_value("true", 0), size_t(4)); }

    test_runner::start_case("skip_value_false");
    { CHECK_EQ(skip_value("false", 0), size_t(5)); }

    test_runner::start_case("skip_value_null");
    { CHECK_EQ(skip_value("null", 0), size_t(4)); }

    test_runner::start_case("skip_value_empty_object");
    { CHECK_EQ(skip_value("{}", 0), size_t(2)); }

    test_runner::start_case("skip_value_empty_array");
    { CHECK_EQ(skip_value("[]", 0), size_t(2)); }

    test_runner::start_case("skip_value_nested_object");
    { CHECK_EQ(skip_value(R"({"a":{"b":1},"c":[1,2]})", 0), size_t(23)); }

    test_runner::start_case("skip_value_array_of_objects");
    { CHECK_EQ(skip_value(R"([{"a":1},{"b":2}])", 0), size_t(17)); }

    test_runner::start_case("skip_value_braces_in_strings");
    {
        // { } inside strings must not unbalance depth.
        const char* s = R"({"a":"}{[]"})";
        CHECK_EQ(skip_value(s, 0), std::string(s).size());
    }

    test_runner::start_case("skip_value_escapes_in_strings");
    {
        // Escaped quote inside string must not end the string early.
        const char* s = R"({"a":"\""})";
        CHECK_EQ(skip_value(s, 0), std::string(s).size());
    }

    test_runner::start_case("skip_value_leading_whitespace");
    { CHECK_EQ(skip_value("   42", 0), size_t(5)); }

    test_runner::start_case("skip_value_rejects_eof");
    { CHECK_EQ(skip_value("", 0), std::string::npos); }

    test_runner::start_case("skip_value_rejects_unbalanced_object");
    { CHECK_EQ(skip_value("{", 0), std::string::npos); }
}

// =====================================================================
// DecodeFrame happy paths.
// =====================================================================

static void test_decode_happy() {
    test_runner::start_case("decode_request_numeric_id");
    {
        const char* s = R"({"jsonrpc":"2.0","method":"set_format","params":{"fourcc":"YUYV","w":1920},"id":42})";
        Frame f;
        std::string err;
        CHECK_TRUE(DecodeFrame(s, f, &err));
        CHECK_TRUE(f.kind == FrameKind::Request);
        CHECK_STR_EQ(f.method, "set_format");
        CHECK_STR_EQ(f.id_raw, "42");
        CHECK_TRUE(f.has_id);
        CHECK_STR_EQ(f.params_json, R"({"fourcc":"YUYV","w":1920})");
        CHECK_TRUE(!f.has_result);
        CHECK_TRUE(!f.has_error);
    }

    test_runner::start_case("decode_request_string_id");
    {
        const char* s = R"({"jsonrpc":"2.0","method":"x","id":"abc-1"})";
        Frame f;
        CHECK_TRUE(DecodeFrame(s, f, nullptr));
        CHECK_TRUE(f.kind == FrameKind::Request);
        CHECK_STR_EQ(f.id_raw, "\"abc-1\"");
    }

    test_runner::start_case("decode_request_null_id");
    {
        // null id is permitted by the spec.
        const char* s = R"({"jsonrpc":"2.0","method":"x","id":null})";
        Frame f;
        CHECK_TRUE(DecodeFrame(s, f, nullptr));
        CHECK_TRUE(f.kind == FrameKind::Request);
        CHECK_STR_EQ(f.id_raw, "null");
    }

    test_runner::start_case("decode_request_negative_id");
    {
        // Unusual but legal — id may be any number.
        const char* s = R"({"jsonrpc":"2.0","method":"x","id":-7})";
        Frame f;
        CHECK_TRUE(DecodeFrame(s, f, nullptr));
        CHECK_STR_EQ(f.id_raw, "-7");
    }

    test_runner::start_case("decode_notification");
    {
        const char* s = R"({"jsonrpc":"2.0","method":"status","params":{"health":"LIVE"}})";
        Frame f;
        CHECK_TRUE(DecodeFrame(s, f, nullptr));
        CHECK_TRUE(f.kind == FrameKind::Notification);
        CHECK_STR_EQ(f.method, "status");
        CHECK_TRUE(!f.has_id);
        CHECK_STR_EQ(f.params_json, R"({"health":"LIVE"})");
    }

    test_runner::start_case("decode_notification_no_params");
    {
        // params is OPTIONAL per JSON-RPC 2.0 §4.
        const char* s = R"({"jsonrpc":"2.0","method":"ping"})";
        Frame f;
        CHECK_TRUE(DecodeFrame(s, f, nullptr));
        CHECK_TRUE(f.kind == FrameKind::Notification);
        CHECK_STR_EQ(f.params_json, "");
    }

    test_runner::start_case("decode_notification_array_params");
    {
        // Per spec §4.2, params may also be an array.
        const char* s = R"({"jsonrpc":"2.0","method":"x","params":[1,2,3]})";
        Frame f;
        CHECK_TRUE(DecodeFrame(s, f, nullptr));
        CHECK_STR_EQ(f.params_json, "[1,2,3]");
    }

    test_runner::start_case("decode_response_success");
    {
        const char* s = R"({"jsonrpc":"2.0","result":{"applied":true},"id":42})";
        Frame f;
        CHECK_TRUE(DecodeFrame(s, f, nullptr));
        CHECK_TRUE(f.kind == FrameKind::Response);
        CHECK_TRUE(f.has_result);
        CHECK_TRUE(!f.has_error);
        CHECK_STR_EQ(f.result_json, R"({"applied":true})");
    }

    test_runner::start_case("decode_response_result_can_be_any_value");
    {
        // result may be number / string / null / array, not just object.
        Frame f;
        CHECK_TRUE(DecodeFrame(R"({"jsonrpc":"2.0","result":42,"id":1})", f, nullptr));
        CHECK_STR_EQ(f.result_json, "42");

        Frame f2;
        CHECK_TRUE(DecodeFrame(R"({"jsonrpc":"2.0","result":"ok","id":1})", f2, nullptr));
        CHECK_STR_EQ(f2.result_json, "\"ok\"");

        Frame f3;
        CHECK_TRUE(DecodeFrame(R"({"jsonrpc":"2.0","result":null,"id":1})", f3, nullptr));
        CHECK_STR_EQ(f3.result_json, "null");
    }

    test_runner::start_case("decode_response_error");
    {
        const char* s = R"({"jsonrpc":"2.0","error":{"code":-32000,"message":"EINVAL"},"id":42})";
        Frame f;
        CHECK_TRUE(DecodeFrame(s, f, nullptr));
        CHECK_TRUE(f.kind == FrameKind::Response);
        CHECK_TRUE(f.has_error);
        CHECK_EQ(f.error_code, int64_t(-32000));
        CHECK_STR_EQ(f.error_message, "EINVAL");
        CHECK_STR_EQ(f.error_data_json, "");
    }

    test_runner::start_case("decode_response_error_with_data_object");
    {
        const char* s =
            R"({"jsonrpc":"2.0","error":{"code":1,"message":"oops","data":{"detail":"x"}},"id":7})";
        Frame f;
        CHECK_TRUE(DecodeFrame(s, f, nullptr));
        CHECK_TRUE(f.has_error);
        CHECK_STR_EQ(f.error_data_json, R"({"detail":"x"})");
    }

    test_runner::start_case("decode_response_error_with_data_string");
    {
        const char* s =
            R"({"jsonrpc":"2.0","error":{"code":1,"message":"oops","data":"hi"},"id":7})";
        Frame f;
        CHECK_TRUE(DecodeFrame(s, f, nullptr));
        CHECK_STR_EQ(f.error_data_json, "\"hi\"");
    }

    test_runner::start_case("decode_response_error_extra_fields_in_error_object");
    {
        // Unknown keys inside error object must be tolerated.
        const char* s =
            R"({"jsonrpc":"2.0","error":{"code":1,"message":"oops","extra":42},"id":7})";
        Frame f;
        std::string err;
        CHECK_TRUE(DecodeFrame(s, f, &err));
        CHECK_EQ(f.error_code, int64_t(1));
    }

    test_runner::start_case("decode_whitespace_tolerance");
    {
        const char* s = "{\n  \"jsonrpc\" : \"2.0\" ,\n  \"method\" : \"x\" ,\n  \"id\" : 1\n}";
        Frame f;
        CHECK_TRUE(DecodeFrame(s, f, nullptr));
        CHECK_TRUE(f.kind == FrameKind::Request);
    }

    test_runner::start_case("decode_field_order_insensitive");
    {
        // id before method before jsonrpc must still work.
        const char* s = R"({"id":1,"method":"x","jsonrpc":"2.0"})";
        Frame f;
        CHECK_TRUE(DecodeFrame(s, f, nullptr));
        CHECK_TRUE(f.kind == FrameKind::Request);
    }

    test_runner::start_case("decode_forward_compat_unknown_top_level_keys");
    {
        const char* s = R"({"jsonrpc":"2.0","method":"x","params":{},"id":1,"future":"hi","also":[1,2],"nested":{"k":"v"}})";
        Frame f;
        std::string err;
        CHECK_TRUE(DecodeFrame(s, f, &err));
        CHECK_TRUE(f.kind == FrameKind::Request);
    }
}

// =====================================================================
// DecodeFrame rejection paths.
// =====================================================================

static void test_decode_rejects() {
    test_runner::start_case("decode_rejects_empty");
    {
        Frame f;
        std::string err;
        CHECK_TRUE(!DecodeFrame("", f, &err));
    }

    test_runner::start_case("decode_rejects_not_object");
    {
        Frame f;
        std::string err;
        CHECK_TRUE(!DecodeFrame("42", f, &err));
        CHECK_TRUE(!DecodeFrame("[]", f, &err));
        CHECK_TRUE(!DecodeFrame("null", f, &err));
        CHECK_TRUE(!DecodeFrame("\"x\"", f, &err));
    }

    test_runner::start_case("decode_rejects_missing_jsonrpc");
    {
        Frame f;
        std::string err;
        CHECK_TRUE(!DecodeFrame(R"({"method":"x","id":1})", f, &err));
    }

    test_runner::start_case("decode_rejects_wrong_jsonrpc_version");
    {
        Frame f;
        std::string err;
        CHECK_TRUE(!DecodeFrame(R"({"jsonrpc":"1.0","method":"x"})", f, &err));
        CHECK_TRUE(!DecodeFrame(R"({"jsonrpc":"2","method":"x"})", f, &err));
    }

    test_runner::start_case("decode_rejects_jsonrpc_not_string");
    {
        Frame f;
        std::string err;
        CHECK_TRUE(!DecodeFrame(R"({"jsonrpc":2.0,"method":"x"})", f, &err));
    }

    test_runner::start_case("decode_rejects_truncated_at_brace");
    {
        Frame f;
        CHECK_TRUE(!DecodeFrame("{", f, nullptr));
    }

    test_runner::start_case("decode_rejects_truncated_after_key");
    {
        Frame f;
        CHECK_TRUE(!DecodeFrame(R"({"jsonrpc")", f, nullptr));
    }

    test_runner::start_case("decode_rejects_truncated_after_colon");
    {
        Frame f;
        CHECK_TRUE(!DecodeFrame(R"({"jsonrpc":)", f, nullptr));
    }

    test_runner::start_case("decode_rejects_truncated_after_value");
    {
        Frame f;
        CHECK_TRUE(!DecodeFrame(R"({"jsonrpc":"2.0")", f, nullptr));
    }

    test_runner::start_case("decode_rejects_truncated_after_comma");
    {
        Frame f;
        CHECK_TRUE(!DecodeFrame(R"({"jsonrpc":"2.0",)", f, nullptr));
    }

    test_runner::start_case("decode_rejects_missing_colon");
    {
        Frame f;
        CHECK_TRUE(!DecodeFrame(R"({"jsonrpc" "2.0"})", f, nullptr));
    }

    test_runner::start_case("decode_rejects_no_method_no_id");
    {
        // Just jsonrpc with no method or id — not a valid frame.
        Frame f;
        std::string err;
        CHECK_TRUE(!DecodeFrame(R"({"jsonrpc":"2.0"})", f, &err));
    }

    test_runner::start_case("decode_rejects_result_and_error_both");
    {
        Frame f;
        std::string err;
        CHECK_TRUE(!DecodeFrame(
            R"({"jsonrpc":"2.0","result":{},"error":{"code":1,"message":"x"},"id":1})", f, &err));
    }

    test_runner::start_case("decode_rejects_response_with_neither_result_nor_error");
    {
        // id but no method, no result, no error.
        Frame f;
        std::string err;
        CHECK_TRUE(!DecodeFrame(R"({"jsonrpc":"2.0","id":1})", f, &err));
    }

    test_runner::start_case("decode_rejects_error_not_object");
    {
        Frame f;
        CHECK_TRUE(!DecodeFrame(R"({"jsonrpc":"2.0","error":"oops","id":1})", f, nullptr));
    }

    test_runner::start_case("decode_rejects_error_bad_code_type");
    {
        Frame f;
        CHECK_TRUE(!DecodeFrame(R"({"jsonrpc":"2.0","error":{"code":"bad","message":"x"},"id":1})",
                                f, nullptr));
    }

    test_runner::start_case("decode_rejects_bad_id_value");
    {
        Frame f;
        CHECK_TRUE(!DecodeFrame(R"({"jsonrpc":"2.0","method":"x","id":})", f, nullptr));
    }

    test_runner::start_case("decode_rejects_bad_method_type");
    {
        Frame f;
        // method must be a string.
        CHECK_TRUE(!DecodeFrame(R"({"jsonrpc":"2.0","method":42,"id":1})", f, nullptr));
    }
}

// =====================================================================
// Encoder roundtrip + spot-checks.
// =====================================================================

static void test_encoders() {
    test_runner::start_case("encode_request_basic");
    {
        std::string s = EncodeRequest("set_format", R"({"fourcc":"YUYV"})", "42");
        CHECK_TRUE(s.find("\"jsonrpc\":\"2.0\"") != std::string::npos);
        CHECK_TRUE(s.find("\"method\":\"set_format\"") != std::string::npos);
        CHECK_TRUE(s.find("\"params\":{\"fourcc\":\"YUYV\"}") != std::string::npos);
        CHECK_TRUE(s.find("\"id\":42") != std::string::npos);
        Frame f;
        CHECK_TRUE(DecodeFrame(s, f, nullptr));
        CHECK_TRUE(f.kind == FrameKind::Request);
        CHECK_STR_EQ(f.method, "set_format");
        CHECK_STR_EQ(f.id_raw, "42");
        CHECK_STR_EQ(f.params_json, R"({"fourcc":"YUYV"})");
    }

    test_runner::start_case("encode_request_no_params_field");
    {
        // When params is empty string, the encoder omits the field
        // entirely (still valid per JSON-RPC 2.0 §4.2).
        std::string s = EncodeRequest("ping", "", "1");
        CHECK_TRUE(s.find("\"params\"") == std::string::npos);
        Frame f;
        CHECK_TRUE(DecodeFrame(s, f, nullptr));
        CHECK_TRUE(f.kind == FrameKind::Request);
        CHECK_STR_EQ(f.params_json, "");
    }

    test_runner::start_case("encode_request_string_id");
    {
        // Caller supplies id_raw verbatim — including quotes for strings.
        std::string s = EncodeRequest("x", "", "\"abc\"");
        Frame f;
        CHECK_TRUE(DecodeFrame(s, f, nullptr));
        CHECK_STR_EQ(f.id_raw, "\"abc\"");
    }

    test_runner::start_case("encode_notification_basic");
    {
        std::string s = EncodeNotification("status", R"({"health":"LIVE"})");
        CHECK_TRUE(s.find("\"jsonrpc\":\"2.0\"") != std::string::npos);
        CHECK_TRUE(s.find("\"method\":\"status\"") != std::string::npos);
        CHECK_TRUE(s.find("\"id\"") == std::string::npos);
        Frame f;
        CHECK_TRUE(DecodeFrame(s, f, nullptr));
        CHECK_TRUE(f.kind == FrameKind::Notification);
    }

    test_runner::start_case("encode_notification_no_params");
    {
        std::string s = EncodeNotification("hello", "");
        CHECK_TRUE(s.find("\"params\"") == std::string::npos);
        Frame f;
        CHECK_TRUE(DecodeFrame(s, f, nullptr));
        CHECK_TRUE(f.kind == FrameKind::Notification);
    }

    test_runner::start_case("encode_response_result_object");
    {
        std::string s = EncodeResponseResult(R"({"applied":true})", "42");
        Frame f;
        CHECK_TRUE(DecodeFrame(s, f, nullptr));
        CHECK_TRUE(f.has_result);
        CHECK_STR_EQ(f.result_json, R"({"applied":true})");
    }

    test_runner::start_case("encode_response_result_empty_becomes_empty_object");
    {
        // Spec §5: result MUST be present on success. Encoder maps "" → "{}".
        std::string s = EncodeResponseResult("", "1");
        CHECK_TRUE(s.find("\"result\":{}") != std::string::npos);
        Frame f;
        CHECK_TRUE(DecodeFrame(s, f, nullptr));
        CHECK_TRUE(f.has_result);
    }

    test_runner::start_case("encode_response_error_basic");
    {
        std::string s = EncodeResponseError(-32000, "EINVAL", "", "42");
        Frame f;
        CHECK_TRUE(DecodeFrame(s, f, nullptr));
        CHECK_TRUE(f.has_error);
        CHECK_EQ(f.error_code, int64_t(-32000));
        CHECK_STR_EQ(f.error_message, "EINVAL");
        CHECK_STR_EQ(f.error_data_json, "");
    }

    test_runner::start_case("encode_response_error_with_data");
    {
        std::string s = EncodeResponseError(1, "oops", R"({"x":1})", "42");
        Frame f;
        CHECK_TRUE(DecodeFrame(s, f, nullptr));
        CHECK_STR_EQ(f.error_data_json, R"({"x":1})");
    }

    test_runner::start_case("encode_escapes_method_with_special_chars");
    {
        std::string s = EncodeNotification(R"(name"with\backslash)", "");
        Frame f;
        std::string err;
        CHECK_TRUE(DecodeFrame(s, f, &err));
        CHECK_STR_EQ(f.method, R"(name"with\backslash)");
    }

    test_runner::start_case("encode_escapes_message_with_special_chars");
    {
        std::string s = EncodeResponseError(1, "line1\nline2\t<tab>", "", "1");
        Frame f;
        CHECK_TRUE(DecodeFrame(s, f, nullptr));
        CHECK_STR_EQ(f.error_message, "line1\nline2\t<tab>");
    }

    test_runner::start_case("encode_escapes_control_chars");
    {
        std::string in;
        in.push_back('\x01');
        in.push_back('\x1f');
        std::string s = EncodeResponseError(1, in, "", "1");
        CHECK_TRUE(s.find("\\u0001") != std::string::npos);
        CHECK_TRUE(s.find("\\u001f") != std::string::npos);
        Frame f;
        CHECK_TRUE(DecodeFrame(s, f, nullptr));
        CHECK_STR_EQ(f.error_message, in);
    }

    test_runner::start_case("decode_unicode_escape_bmp");
    {
        // é is é (U+00E9). UTF-8: 0xc3 0xa9.
        const char* s = R"({"jsonrpc":"2.0","method":"café"})";
        Frame f;
        CHECK_TRUE(DecodeFrame(s, f, nullptr));
        CHECK_STR_EQ(f.method, "caf\xc3\xa9");
    }

    test_runner::start_case("decode_unicode_escape_surrogate_pair");
    {
        // U+1F600 (😀) encodes as surrogate pair 😀; UTF-8 is F0 9F 98 80.
        const char* s = R"({"jsonrpc":"2.0","method":"😀"})";
        Frame f;
        std::string err;
        CHECK_TRUE(DecodeFrame(s, f, &err));
        CHECK_STR_EQ(f.method, "\xf0\x9f\x98\x80");
    }

    test_runner::start_case("decode_unicode_escape_invalid_hex_rejected");
    {
        Frame f;
        CHECK_TRUE(!DecodeFrame(R"({"jsonrpc":"2.0","method":"\u00ZZ"})", f, nullptr));
    }

    test_runner::start_case("decode_unicode_escape_truncated_rejected");
    {
        Frame f;
        CHECK_TRUE(!DecodeFrame(R"({"jsonrpc":"2.0","method":"\u00)", f, nullptr));
    }

    test_runner::start_case("encode_decode_string_with_quote");
    {
        // " inside a string field must round-trip.
        std::string s = EncodeResponseError(1, R"(say "hi")", "", "1");
        Frame f;
        CHECK_TRUE(DecodeFrame(s, f, nullptr));
        CHECK_STR_EQ(f.error_message, R"(say "hi")");
    }

    test_runner::start_case("encode_decode_backslash_in_string");
    {
        std::string s = EncodeResponseError(1, R"(a\b)", "", "1");
        Frame f;
        CHECK_TRUE(DecodeFrame(s, f, nullptr));
        CHECK_STR_EQ(f.error_message, R"(a\b)");
    }
}

int main() {
    test_skip_ws();
    test_parse_string();
    test_parse_uint();
    test_parse_int();
    test_parse_uint_array();
    test_skip_value();
    test_decode_happy();
    test_decode_rejects();
    test_encoders();
    return test_runner::report_and_exit_code();
}
