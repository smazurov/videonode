#include "src/rpc/jsonrpc_msg.hpp"

#include <gtest/gtest.h>

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

TEST(JsonrpcParse, SkipWsEmpty) {
    EXPECT_EQ(skip_ws("", 0), size_t(0));
}

TEST(JsonrpcParse, SkipWsNoWhitespace) {
    EXPECT_EQ(skip_ws("abc", 0), size_t(0));
}

TEST(JsonrpcParse, SkipWsSpaces) {
    EXPECT_EQ(skip_ws("   a", 0), size_t(3));
}

TEST(JsonrpcParse, SkipWsTabsNewlinesReturns) {
    EXPECT_EQ(skip_ws("\t\n\r a", 0), size_t(4));
}

TEST(JsonrpcParse, SkipWsFromMiddle) {
    // " ab   cd" — start at index 3, should skip to 'c' at 6.
    EXPECT_EQ(skip_ws(" ab   cd", 3), size_t(6));
}

TEST(JsonrpcParse, SkipWsOnlyWhitespace) {
    EXPECT_EQ(skip_ws("    ", 0), size_t(4));
}

TEST(JsonrpcParse, ParseStringEmpty) {
    std::string out;
    EXPECT_EQ(parse_string("\"\"", 0, out), size_t(2));
    EXPECT_EQ(out, "");
}

TEST(JsonrpcParse, ParseStringBasic) {
    std::string out;
    EXPECT_EQ(parse_string("\"hello\"", 0, out), size_t(7));
    EXPECT_EQ(out, "hello");
}

TEST(JsonrpcParse, ParseStringEscapeQuote) {
    std::string out;
    const char* s = R"("a\"b")";
    EXPECT_EQ(parse_string(s, 0, out), std::string(s).size());
    EXPECT_EQ(out, "a\"b");
}

TEST(JsonrpcParse, ParseStringEscapeBackslash) {
    std::string out;
    const char* s = R"("a\\b")";
    EXPECT_EQ(parse_string(s, 0, out), std::string(s).size());
    EXPECT_EQ(out, "a\\b");
}

TEST(JsonrpcParse, ParseStringEscapeSlash) {
    std::string out;
    const char* s = R"("a\/b")";
    EXPECT_EQ(parse_string(s, 0, out), std::string(s).size());
    EXPECT_EQ(out, "a/b");
}

TEST(JsonrpcParse, ParseStringEscapeNTR) {
    std::string out;
    const char* s = R"("a\nb\tc\rd")";
    EXPECT_EQ(parse_string(s, 0, out), std::string(s).size());
    EXPECT_EQ(out, "a\nb\tc\rd");
}

TEST(JsonrpcParse, ParseStringRejectsMissingOpenQuote) {
    std::string out;
    EXPECT_EQ(parse_string("abc", 0, out), std::string::npos);
}

TEST(JsonrpcParse, ParseStringRejectsUnterminated) {
    std::string out;
    EXPECT_EQ(parse_string("\"abc", 0, out), std::string::npos);
}

TEST(JsonrpcParse, ParseStringRejectsEofAfterOpenQuote) {
    std::string out;
    EXPECT_EQ(parse_string("\"", 0, out), std::string::npos);
}

TEST(JsonrpcParse, ParseUintZero) {
    uint64_t v = 99;
    EXPECT_EQ(parse_uint("0", 0, v), size_t(1));
    EXPECT_EQ(v, uint64_t(0));
}

TEST(JsonrpcParse, ParseUintBasic) {
    uint64_t v = 0;
    EXPECT_EQ(parse_uint("12345", 0, v), size_t(5));
    EXPECT_EQ(v, uint64_t(12345));
}

TEST(JsonrpcParse, ParseUintLarge) {
    uint64_t v = 0;
    EXPECT_EQ(parse_uint("18446744073709551615", 0, v), size_t(20));
    EXPECT_EQ(v, UINT64_MAX);
}

TEST(JsonrpcParse, ParseUintStopsAtNondigit) {
    uint64_t v = 0;
    EXPECT_EQ(parse_uint("42abc", 0, v), size_t(2));
    EXPECT_EQ(v, uint64_t(42));
}

TEST(JsonrpcParse, ParseUintRejectsNegative) {
    uint64_t v = 0;
    EXPECT_EQ(parse_uint("-1", 0, v), std::string::npos);
}

TEST(JsonrpcParse, ParseUintRejectsNonDigit) {
    uint64_t v = 0;
    EXPECT_EQ(parse_uint("abc", 0, v), std::string::npos);
}

TEST(JsonrpcParse, ParseUintRejectsEmpty) {
    uint64_t v = 0;
    EXPECT_EQ(parse_uint("", 0, v), std::string::npos);
}

TEST(JsonrpcParse, ParseIntPositive) {
    int64_t v = 0;
    EXPECT_EQ(parse_int("42", 0, v), size_t(2));
    EXPECT_EQ(v, int64_t(42));
}

TEST(JsonrpcParse, ParseIntNegative) {
    int64_t v = 0;
    EXPECT_EQ(parse_int("-42", 0, v), size_t(3));
    EXPECT_EQ(v, int64_t(-42));
}

TEST(JsonrpcParse, ParseIntNegativeJsonrpcErrorCode) {
    int64_t v = 0;
    EXPECT_EQ(parse_int("-32700", 0, v), size_t(6));
    EXPECT_EQ(v, int64_t(-32700));
}

TEST(JsonrpcParse, ParseIntZero) {
    int64_t v = 1;
    EXPECT_EQ(parse_int("0", 0, v), size_t(1));
    EXPECT_EQ(v, int64_t(0));
}

TEST(JsonrpcParse, ParseIntRejectsSoloMinus) {
    int64_t v = 0;
    EXPECT_EQ(parse_int("-", 0, v), std::string::npos);
}

TEST(JsonrpcParse, ParseUintArrayEmpty) {
    std::vector<uint32_t> a{99};
    EXPECT_EQ(parse_uint_array("[]", 0, a), size_t(2));
    EXPECT_EQ(a.size(), size_t(0));
}

TEST(JsonrpcParse, ParseUintArraySingle) {
    std::vector<uint32_t> a;
    EXPECT_EQ(parse_uint_array("[42]", 0, a), size_t(4));
    EXPECT_EQ(a.size(), size_t(1));
    EXPECT_EQ(a[0], uint32_t(42));
}

TEST(JsonrpcParse, ParseUintArrayMulti) {
    std::vector<uint32_t> a;
    EXPECT_EQ(parse_uint_array("[1,2,3]", 0, a), size_t(7));
    EXPECT_EQ(a.size(), size_t(3));
    EXPECT_EQ(a[0], uint32_t(1));
    EXPECT_EQ(a[1], uint32_t(2));
    EXPECT_EQ(a[2], uint32_t(3));
}

TEST(JsonrpcParse, ParseUintArraySpaces) {
    std::vector<uint32_t> a;
    EXPECT_EQ(parse_uint_array("[ 1 , 2 , 3 ]", 0, a), size_t(13));
    EXPECT_EQ(a.size(), size_t(3));
}

TEST(JsonrpcParse, ParseUintArrayRejectsMissingBracket) {
    std::vector<uint32_t> a;
    EXPECT_EQ(parse_uint_array("1,2,3", 0, a), std::string::npos);
}

TEST(JsonrpcParse, ParseUintArrayRejectsUnterminated) {
    std::vector<uint32_t> a;
    EXPECT_EQ(parse_uint_array("[1,2", 0, a), std::string::npos);
}

TEST(JsonrpcParse, ParseUintArrayRejectsTrailingComma) {
    std::vector<uint32_t> a;
    EXPECT_EQ(parse_uint_array("[1,]", 0, a), std::string::npos);
}

TEST(JsonrpcParse, SkipValueString) {
    EXPECT_EQ(skip_value(R"("abc")", 0), size_t(5));
}

TEST(JsonrpcParse, SkipValueStringWithEscape) {
    // R"("a\"b")" is 6 characters: " a \ " b "
    EXPECT_EQ(skip_value(R"("a\"b")", 0), size_t(6));
}

TEST(JsonrpcParse, SkipValueUint) {
    EXPECT_EQ(skip_value("42", 0), size_t(2));
}

TEST(JsonrpcParse, SkipValueIntNegative) {
    EXPECT_EQ(skip_value("-42", 0), size_t(3));
}

TEST(JsonrpcParse, SkipValueFloat) {
    EXPECT_EQ(skip_value("3.14", 0), size_t(4));
}

TEST(JsonrpcParse, SkipValueExponent) {
    EXPECT_EQ(skip_value("1e10", 0), size_t(4));
}

TEST(JsonrpcParse, SkipValueNegExponent) {
    EXPECT_EQ(skip_value("1.5e-3", 0), size_t(6));
}

TEST(JsonrpcParse, SkipValueTrue) {
    EXPECT_EQ(skip_value("true", 0), size_t(4));
}

TEST(JsonrpcParse, SkipValueFalse) {
    EXPECT_EQ(skip_value("false", 0), size_t(5));
}

TEST(JsonrpcParse, SkipValueNull) {
    EXPECT_EQ(skip_value("null", 0), size_t(4));
}

TEST(JsonrpcParse, SkipValueEmptyObject) {
    EXPECT_EQ(skip_value("{}", 0), size_t(2));
}

TEST(JsonrpcParse, SkipValueEmptyArray) {
    EXPECT_EQ(skip_value("[]", 0), size_t(2));
}

TEST(JsonrpcParse, SkipValueNestedObject) {
    EXPECT_EQ(skip_value(R"({"a":{"b":1},"c":[1,2]})", 0), size_t(23));
}

TEST(JsonrpcParse, SkipValueArrayOfObjects) {
    EXPECT_EQ(skip_value(R"([{"a":1},{"b":2}])", 0), size_t(17));
}

TEST(JsonrpcParse, SkipValueBracesInStrings) {
    // { } inside strings must not unbalance depth.
    const char* s = R"({"a":"}{[]"})";
    EXPECT_EQ(skip_value(s, 0), std::string(s).size());
}

TEST(JsonrpcParse, SkipValueEscapesInStrings) {
    // Escaped quote inside string must not end the string early.
    const char* s = R"({"a":"\""})";
    EXPECT_EQ(skip_value(s, 0), std::string(s).size());
}

TEST(JsonrpcParse, SkipValueLeadingWhitespace) {
    EXPECT_EQ(skip_value("   42", 0), size_t(5));
}

TEST(JsonrpcParse, SkipValueRejectsEof) {
    EXPECT_EQ(skip_value("", 0), std::string::npos);
}

TEST(JsonrpcParse, SkipValueRejectsUnbalancedObject) {
    EXPECT_EQ(skip_value("{", 0), std::string::npos);
}

// =====================================================================
// DecodeFrame happy paths.
// =====================================================================

TEST(JsonrpcDecode, RequestNumericId) {
    const char* s =
        R"({"jsonrpc":"2.0","method":"set_format","params":{"fourcc":"YUYV","w":1920},"id":42})";
    Frame f;
    std::string err;
    EXPECT_TRUE(DecodeFrame(s, f, &err));
    EXPECT_TRUE(f.kind == FrameKind::Request);
    EXPECT_EQ(f.method, "set_format");
    EXPECT_EQ(f.id_raw, "42");
    EXPECT_TRUE(f.has_id);
    EXPECT_EQ(f.params_json, R"({"fourcc":"YUYV","w":1920})");
    EXPECT_FALSE(f.has_result);
    EXPECT_FALSE(f.has_error);
}

TEST(JsonrpcDecode, RequestStringId) {
    const char* s = R"({"jsonrpc":"2.0","method":"x","id":"abc-1"})";
    Frame f;
    EXPECT_TRUE(DecodeFrame(s, f, nullptr));
    EXPECT_TRUE(f.kind == FrameKind::Request);
    EXPECT_EQ(f.id_raw, "\"abc-1\"");
}

TEST(JsonrpcDecode, RequestNullId) {
    // null id is permitted by the spec.
    const char* s = R"({"jsonrpc":"2.0","method":"x","id":null})";
    Frame f;
    EXPECT_TRUE(DecodeFrame(s, f, nullptr));
    EXPECT_TRUE(f.kind == FrameKind::Request);
    EXPECT_EQ(f.id_raw, "null");
}

TEST(JsonrpcDecode, RequestNegativeId) {
    // Unusual but legal — id may be any number.
    const char* s = R"({"jsonrpc":"2.0","method":"x","id":-7})";
    Frame f;
    EXPECT_TRUE(DecodeFrame(s, f, nullptr));
    EXPECT_EQ(f.id_raw, "-7");
}

TEST(JsonrpcDecode, Notification) {
    const char* s = R"({"jsonrpc":"2.0","method":"status","params":{"health":"LIVE"}})";
    Frame f;
    EXPECT_TRUE(DecodeFrame(s, f, nullptr));
    EXPECT_TRUE(f.kind == FrameKind::Notification);
    EXPECT_EQ(f.method, "status");
    EXPECT_FALSE(f.has_id);
    EXPECT_EQ(f.params_json, R"({"health":"LIVE"})");
}

TEST(JsonrpcDecode, NotificationNoParams) {
    // params is OPTIONAL per JSON-RPC 2.0 §4.
    const char* s = R"({"jsonrpc":"2.0","method":"ping"})";
    Frame f;
    EXPECT_TRUE(DecodeFrame(s, f, nullptr));
    EXPECT_TRUE(f.kind == FrameKind::Notification);
    EXPECT_EQ(f.params_json, "");
}

TEST(JsonrpcDecode, NotificationArrayParams) {
    // Per spec §4.2, params may also be an array.
    const char* s = R"({"jsonrpc":"2.0","method":"x","params":[1,2,3]})";
    Frame f;
    EXPECT_TRUE(DecodeFrame(s, f, nullptr));
    EXPECT_EQ(f.params_json, "[1,2,3]");
}

TEST(JsonrpcDecode, ResponseSuccess) {
    const char* s = R"({"jsonrpc":"2.0","result":{"applied":true},"id":42})";
    Frame f;
    EXPECT_TRUE(DecodeFrame(s, f, nullptr));
    EXPECT_TRUE(f.kind == FrameKind::Response);
    EXPECT_TRUE(f.has_result);
    EXPECT_FALSE(f.has_error);
    EXPECT_EQ(f.result_json, R"({"applied":true})");
}

TEST(JsonrpcDecode, ResponseResultCanBeAnyValue) {
    // result may be number / string / null / array, not just object.
    Frame f;
    EXPECT_TRUE(DecodeFrame(R"({"jsonrpc":"2.0","result":42,"id":1})", f, nullptr));
    EXPECT_EQ(f.result_json, "42");

    Frame f2;
    EXPECT_TRUE(DecodeFrame(R"({"jsonrpc":"2.0","result":"ok","id":1})", f2, nullptr));
    EXPECT_EQ(f2.result_json, "\"ok\"");

    Frame f3;
    EXPECT_TRUE(DecodeFrame(R"({"jsonrpc":"2.0","result":null,"id":1})", f3, nullptr));
    EXPECT_EQ(f3.result_json, "null");
}

TEST(JsonrpcDecode, ResponseError) {
    const char* s = R"({"jsonrpc":"2.0","error":{"code":-32000,"message":"EINVAL"},"id":42})";
    Frame f;
    EXPECT_TRUE(DecodeFrame(s, f, nullptr));
    EXPECT_TRUE(f.kind == FrameKind::Response);
    EXPECT_TRUE(f.has_error);
    EXPECT_EQ(f.error_code, int64_t(-32000));
    EXPECT_EQ(f.error_message, "EINVAL");
    EXPECT_EQ(f.error_data_json, "");
}

TEST(JsonrpcDecode, ResponseErrorWithDataObject) {
    const char* s =
        R"({"jsonrpc":"2.0","error":{"code":1,"message":"oops","data":{"detail":"x"}},"id":7})";
    Frame f;
    EXPECT_TRUE(DecodeFrame(s, f, nullptr));
    EXPECT_TRUE(f.has_error);
    EXPECT_EQ(f.error_data_json, R"({"detail":"x"})");
}

TEST(JsonrpcDecode, ResponseErrorWithDataString) {
    const char* s = R"({"jsonrpc":"2.0","error":{"code":1,"message":"oops","data":"hi"},"id":7})";
    Frame f;
    EXPECT_TRUE(DecodeFrame(s, f, nullptr));
    EXPECT_EQ(f.error_data_json, "\"hi\"");
}

TEST(JsonrpcDecode, ResponseErrorExtraFieldsInErrorObject) {
    // Unknown keys inside error object must be tolerated.
    const char* s = R"({"jsonrpc":"2.0","error":{"code":1,"message":"oops","extra":42},"id":7})";
    Frame f;
    std::string err;
    EXPECT_TRUE(DecodeFrame(s, f, &err));
    EXPECT_EQ(f.error_code, int64_t(1));
}

TEST(JsonrpcDecode, WhitespaceTolerance) {
    const char* s = "{\n  \"jsonrpc\" : \"2.0\" ,\n  \"method\" : \"x\" ,\n  \"id\" : 1\n}";
    Frame f;
    EXPECT_TRUE(DecodeFrame(s, f, nullptr));
    EXPECT_TRUE(f.kind == FrameKind::Request);
}

TEST(JsonrpcDecode, FieldOrderInsensitive) {
    // id before method before jsonrpc must still work.
    const char* s = R"({"id":1,"method":"x","jsonrpc":"2.0"})";
    Frame f;
    EXPECT_TRUE(DecodeFrame(s, f, nullptr));
    EXPECT_TRUE(f.kind == FrameKind::Request);
}

TEST(JsonrpcDecode, ForwardCompatUnknownTopLevelKeys) {
    const char* s =
        R"({"jsonrpc":"2.0","method":"x","params":{},"id":1,"future":"hi","also":[1,2],"nested":{"k":"v"}})";
    Frame f;
    std::string err;
    EXPECT_TRUE(DecodeFrame(s, f, &err));
    EXPECT_TRUE(f.kind == FrameKind::Request);
}

// =====================================================================
// DecodeFrame rejection paths.
// =====================================================================

TEST(JsonrpcReject, Empty) {
    Frame f;
    std::string err;
    EXPECT_FALSE(DecodeFrame("", f, &err));
}

TEST(JsonrpcReject, NotObject) {
    Frame f;
    std::string err;
    EXPECT_FALSE(DecodeFrame("42", f, &err));
    EXPECT_FALSE(DecodeFrame("[]", f, &err));
    EXPECT_FALSE(DecodeFrame("null", f, &err));
    EXPECT_FALSE(DecodeFrame("\"x\"", f, &err));
}

TEST(JsonrpcReject, MissingJsonrpc) {
    Frame f;
    std::string err;
    EXPECT_FALSE(DecodeFrame(R"({"method":"x","id":1})", f, &err));
}

TEST(JsonrpcReject, WrongJsonrpcVersion) {
    Frame f;
    std::string err;
    EXPECT_FALSE(DecodeFrame(R"({"jsonrpc":"1.0","method":"x"})", f, &err));
    EXPECT_FALSE(DecodeFrame(R"({"jsonrpc":"2","method":"x"})", f, &err));
}

TEST(JsonrpcReject, JsonrpcNotString) {
    Frame f;
    std::string err;
    EXPECT_FALSE(DecodeFrame(R"({"jsonrpc":2.0,"method":"x"})", f, &err));
}

TEST(JsonrpcReject, TruncatedAtBrace) {
    Frame f;
    EXPECT_FALSE(DecodeFrame("{", f, nullptr));
}

TEST(JsonrpcReject, TruncatedAfterKey) {
    Frame f;
    EXPECT_FALSE(DecodeFrame(R"({"jsonrpc")", f, nullptr));
}

TEST(JsonrpcReject, TruncatedAfterColon) {
    Frame f;
    EXPECT_FALSE(DecodeFrame(R"({"jsonrpc":)", f, nullptr));
}

TEST(JsonrpcReject, TruncatedAfterValue) {
    Frame f;
    EXPECT_FALSE(DecodeFrame(R"({"jsonrpc":"2.0")", f, nullptr));
}

TEST(JsonrpcReject, TruncatedAfterComma) {
    Frame f;
    EXPECT_FALSE(DecodeFrame(R"({"jsonrpc":"2.0",)", f, nullptr));
}

TEST(JsonrpcReject, MissingColon) {
    Frame f;
    EXPECT_FALSE(DecodeFrame(R"({"jsonrpc" "2.0"})", f, nullptr));
}

TEST(JsonrpcReject, NoMethodNoId) {
    // Just jsonrpc with no method or id — not a valid frame.
    Frame f;
    std::string err;
    EXPECT_FALSE(DecodeFrame(R"({"jsonrpc":"2.0"})", f, &err));
}

TEST(JsonrpcReject, ResultAndErrorBoth) {
    Frame f;
    std::string err;
    EXPECT_FALSE(DecodeFrame(
        R"({"jsonrpc":"2.0","result":{},"error":{"code":1,"message":"x"},"id":1})", f, &err));
}

TEST(JsonrpcReject, ResponseWithNeitherResultNorError) {
    // id but no method, no result, no error.
    Frame f;
    std::string err;
    EXPECT_FALSE(DecodeFrame(R"({"jsonrpc":"2.0","id":1})", f, &err));
}

TEST(JsonrpcReject, ErrorNotObject) {
    Frame f;
    EXPECT_FALSE(DecodeFrame(R"({"jsonrpc":"2.0","error":"oops","id":1})", f, nullptr));
}

TEST(JsonrpcReject, ErrorBadCodeType) {
    Frame f;
    EXPECT_FALSE(DecodeFrame(R"({"jsonrpc":"2.0","error":{"code":"bad","message":"x"},"id":1})", f,
                             nullptr));
}

TEST(JsonrpcReject, BadIdValue) {
    Frame f;
    EXPECT_FALSE(DecodeFrame(R"({"jsonrpc":"2.0","method":"x","id":})", f, nullptr));
}

TEST(JsonrpcReject, BadMethodType) {
    Frame f;
    // method must be a string.
    EXPECT_FALSE(DecodeFrame(R"({"jsonrpc":"2.0","method":42,"id":1})", f, nullptr));
}

// =====================================================================
// Encoder roundtrip + spot-checks.
// =====================================================================

TEST(JsonrpcEncode, RequestBasic) {
    std::string s = EncodeRequest("set_format", R"({"fourcc":"YUYV"})", "42");
    EXPECT_TRUE(s.find("\"jsonrpc\":\"2.0\"") != std::string::npos);
    EXPECT_TRUE(s.find("\"method\":\"set_format\"") != std::string::npos);
    EXPECT_TRUE(s.find("\"params\":{\"fourcc\":\"YUYV\"}") != std::string::npos);
    EXPECT_TRUE(s.find("\"id\":42") != std::string::npos);
    Frame f;
    EXPECT_TRUE(DecodeFrame(s, f, nullptr));
    EXPECT_TRUE(f.kind == FrameKind::Request);
    EXPECT_EQ(f.method, "set_format");
    EXPECT_EQ(f.id_raw, "42");
    EXPECT_EQ(f.params_json, R"({"fourcc":"YUYV"})");
}

TEST(JsonrpcEncode, RequestNoParamsField) {
    // When params is empty string, the encoder omits the field
    // entirely (still valid per JSON-RPC 2.0 §4.2).
    std::string s = EncodeRequest("ping", "", "1");
    EXPECT_TRUE(s.find("\"params\"") == std::string::npos);
    Frame f;
    EXPECT_TRUE(DecodeFrame(s, f, nullptr));
    EXPECT_TRUE(f.kind == FrameKind::Request);
    EXPECT_EQ(f.params_json, "");
}

TEST(JsonrpcEncode, RequestStringId) {
    // Caller supplies id_raw verbatim — including quotes for strings.
    std::string s = EncodeRequest("x", "", "\"abc\"");
    Frame f;
    EXPECT_TRUE(DecodeFrame(s, f, nullptr));
    EXPECT_EQ(f.id_raw, "\"abc\"");
}

TEST(JsonrpcEncode, NotificationBasic) {
    std::string s = EncodeNotification("status", R"({"health":"LIVE"})");
    EXPECT_TRUE(s.find("\"jsonrpc\":\"2.0\"") != std::string::npos);
    EXPECT_TRUE(s.find("\"method\":\"status\"") != std::string::npos);
    EXPECT_TRUE(s.find("\"id\"") == std::string::npos);
    Frame f;
    EXPECT_TRUE(DecodeFrame(s, f, nullptr));
    EXPECT_TRUE(f.kind == FrameKind::Notification);
}

TEST(JsonrpcEncode, NotificationNoParams) {
    std::string s = EncodeNotification("hello", "");
    EXPECT_TRUE(s.find("\"params\"") == std::string::npos);
    Frame f;
    EXPECT_TRUE(DecodeFrame(s, f, nullptr));
    EXPECT_TRUE(f.kind == FrameKind::Notification);
}

TEST(JsonrpcEncode, ResponseResultObject) {
    std::string s = EncodeResponseResult(R"({"applied":true})", "42");
    Frame f;
    EXPECT_TRUE(DecodeFrame(s, f, nullptr));
    EXPECT_TRUE(f.has_result);
    EXPECT_EQ(f.result_json, R"({"applied":true})");
}

TEST(JsonrpcEncode, ResponseResultEmptyBecomesEmptyObject) {
    // Spec §5: result MUST be present on success. Encoder maps "" → "{}".
    std::string s = EncodeResponseResult("", "1");
    EXPECT_TRUE(s.find("\"result\":{}") != std::string::npos);
    Frame f;
    EXPECT_TRUE(DecodeFrame(s, f, nullptr));
    EXPECT_TRUE(f.has_result);
}

TEST(JsonrpcEncode, ResponseErrorBasic) {
    std::string s = EncodeResponseError(-32000, "EINVAL", "", "42");
    Frame f;
    EXPECT_TRUE(DecodeFrame(s, f, nullptr));
    EXPECT_TRUE(f.has_error);
    EXPECT_EQ(f.error_code, int64_t(-32000));
    EXPECT_EQ(f.error_message, "EINVAL");
    EXPECT_EQ(f.error_data_json, "");
}

TEST(JsonrpcEncode, ResponseErrorWithData) {
    std::string s = EncodeResponseError(1, "oops", R"({"x":1})", "42");
    Frame f;
    EXPECT_TRUE(DecodeFrame(s, f, nullptr));
    EXPECT_EQ(f.error_data_json, R"({"x":1})");
}

TEST(JsonrpcEncode, EscapesMethodWithSpecialChars) {
    std::string s = EncodeNotification(R"(name"with\backslash)", "");
    Frame f;
    std::string err;
    EXPECT_TRUE(DecodeFrame(s, f, &err));
    EXPECT_EQ(f.method, R"(name"with\backslash)");
}

TEST(JsonrpcEncode, EscapesMessageWithSpecialChars) {
    std::string s = EncodeResponseError(1, "line1\nline2\t<tab>", "", "1");
    Frame f;
    EXPECT_TRUE(DecodeFrame(s, f, nullptr));
    EXPECT_EQ(f.error_message, "line1\nline2\t<tab>");
}

TEST(JsonrpcEncode, EscapesControlChars) {
    std::string in;
    in.push_back('\x01');
    in.push_back('\x1f');
    std::string s = EncodeResponseError(1, in, "", "1");
    EXPECT_TRUE(s.find("\\u0001") != std::string::npos);
    EXPECT_TRUE(s.find("\\u001f") != std::string::npos);
    Frame f;
    EXPECT_TRUE(DecodeFrame(s, f, nullptr));
    EXPECT_EQ(f.error_message, in);
}

TEST(JsonrpcDecode, UnicodeEscapeBmp) {
    // é is é (U+00E9). UTF-8: 0xc3 0xa9.
    const char* s = R"({"jsonrpc":"2.0","method":"café"})";
    Frame f;
    EXPECT_TRUE(DecodeFrame(s, f, nullptr));
    EXPECT_EQ(f.method, "caf\xc3\xa9");
}

TEST(JsonrpcDecode, UnicodeEscapeSurrogatePair) {
    // U+1F600 (😀) encodes as surrogate pair 😀; UTF-8 is F0 9F 98 80.
    const char* s = R"({"jsonrpc":"2.0","method":"😀"})";
    Frame f;
    std::string err;
    EXPECT_TRUE(DecodeFrame(s, f, &err));
    EXPECT_EQ(f.method, "\xf0\x9f\x98\x80");
}

TEST(JsonrpcDecode, UnicodeEscapeInvalidHexRejected) {
    Frame f;
    EXPECT_FALSE(DecodeFrame(R"({"jsonrpc":"2.0","method":"\u00ZZ"})", f, nullptr));
}

TEST(JsonrpcDecode, UnicodeEscapeTruncatedRejected) {
    Frame f;
    EXPECT_FALSE(DecodeFrame(R"({"jsonrpc":"2.0","method":"\u00)", f, nullptr));
}

TEST(JsonrpcEncode, RoundtripStringWithQuote) {
    // " inside a string field must round-trip.
    std::string s = EncodeResponseError(1, R"(say "hi")", "", "1");
    Frame f;
    EXPECT_TRUE(DecodeFrame(s, f, nullptr));
    EXPECT_EQ(f.error_message, R"(say "hi")");
}

TEST(JsonrpcEncode, RoundtripBackslashInString) {
    std::string s = EncodeResponseError(1, R"(a\b)", "", "1");
    Frame f;
    EXPECT_TRUE(DecodeFrame(s, f, nullptr));
    EXPECT_EQ(f.error_message, R"(a\b)");
}
