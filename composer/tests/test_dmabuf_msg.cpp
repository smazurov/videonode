#include "src/rpc/dmabuf_msg.hpp"

#include <gtest/gtest.h>

using namespace dmabuf_msg;

TEST(DmabufMsg, EncodeDecodeRoundtripSinglePlane) {
    Header h;
    h.slot_index = 1;
    h.width = 1920;
    h.height = 1080;
    h.format = "NV12";
    h.plane_pitches = {1920};
    h.plane_offsets = {0};
    h.color_matrix = ColorMatrix::Bt601;
    h.color_range = ColorRange::Limited;
    h.chroma_siting = ChromaSiting::Mpeg2;
    h.frame_idx = 42;

    std::string s = EncodeFrameNotification(h);
    // Envelope sentinels.
    EXPECT_TRUE(s.find("\"jsonrpc\":\"2.0\"") != std::string::npos);
    EXPECT_TRUE(s.find("\"method\":\"frame\"") != std::string::npos);
    // No id (it's a notification).
    EXPECT_TRUE(s.find("\"id\"") == std::string::npos);
    // Inner params sentinels.
    EXPECT_TRUE(s.find("\"slot_index\":1") != std::string::npos);
    EXPECT_TRUE(s.find("\"format\":\"NV12\"") != std::string::npos);
    EXPECT_TRUE(s.find("\"plane_pitches\":[1920]") != std::string::npos);
    EXPECT_TRUE(s.find("\"color_matrix\":1") != std::string::npos);
    EXPECT_TRUE(s.find("\"color_range\":1") != std::string::npos);
    EXPECT_TRUE(s.find("\"chroma_siting\":1") != std::string::npos);

    Header back;
    std::string err;
    EXPECT_TRUE(DecodeFrameNotification(s, back, &err));
    EXPECT_EQ(back.slot_index, 1u);
    EXPECT_EQ(back.width, 1920u);
    EXPECT_EQ(back.height, 1080u);
    EXPECT_EQ(back.format, "NV12");
    EXPECT_EQ(back.plane_pitches.size(), 1u);
    EXPECT_EQ(back.plane_pitches[0], 1920u);
    EXPECT_EQ(back.plane_offsets.size(), 1u);
    EXPECT_EQ(back.plane_offsets[0], 0u);
    EXPECT_TRUE(back.color_matrix == ColorMatrix::Bt601);
    EXPECT_TRUE(back.color_range == ColorRange::Limited);
    EXPECT_TRUE(back.chroma_siting == ChromaSiting::Mpeg2);
    EXPECT_EQ(back.frame_idx, 42u);
}

TEST(DmabufMsg, DecodeMissingColorMetadataDefaultsToUnspecified) {
    // Forward-compat: older producers (or hand-rolled tests) may
    // omit the color metadata fields. Decoder must accept and leave
    // them at Unspecified so the consumer can apply its fallback.
    const char* old_style =
        R"({"jsonrpc":"2.0","method":"frame","params":{"slot_index":0,"width":640,"height":480,"format":"NV12","plane_pitches":[640],"plane_offsets":[0],"frame_idx":1}})";
    Header h;
    std::string err;
    EXPECT_TRUE(DecodeFrameNotification(old_style, h, &err));
    EXPECT_TRUE(h.color_matrix == ColorMatrix::Unspecified);
    EXPECT_TRUE(h.color_range == ColorRange::Unspecified);
    EXPECT_TRUE(h.chroma_siting == ChromaSiting::Unspecified);
}

TEST(DmabufMsg, EncodeDecodeRoundtripMultiPlane) {
    Header h;
    h.slot_index = 0;
    h.width = 1280;
    h.height = 720;
    h.format = "NV12";
    h.plane_pitches = {1280, 1280};
    h.plane_offsets = {0, 0};
    h.frame_idx = 1;
    std::string s = EncodeFrameNotification(h);

    Header back;
    EXPECT_TRUE(DecodeFrameNotification(s, back, nullptr));
    EXPECT_EQ(back.plane_pitches.size(), 2u);
    EXPECT_EQ(back.plane_pitches[0], 1280u);
    EXPECT_EQ(back.plane_pitches[1], 1280u);
    EXPECT_EQ(back.plane_offsets.size(), 2u);
}

TEST(DmabufMsg, DecodesGoProducedEnvelope) {
    // Sample JSON-RPC notification shaped exactly like what Go's
    // encoding/json would produce from the equivalent struct on the
    // daemon side.
    const char* go =
        R"({"jsonrpc":"2.0","method":"frame","params":{"slot_index":2,"width":3840,"height":2160,"format":"NV12","plane_pitches":[3840],"plane_offsets":[0],"frame_idx":99}})";
    Header back;
    std::string err;
    EXPECT_TRUE(DecodeFrameNotification(go, back, &err));
    EXPECT_EQ(back.slot_index, 2u);
    EXPECT_EQ(back.width, 3840u);
    EXPECT_EQ(back.height, 2160u);
    EXPECT_EQ(back.format, "NV12");
    EXPECT_EQ(back.frame_idx, 99u);
}

TEST(DmabufMsg, RejectsMalformed) {
    Header h;
    std::string err;
    EXPECT_FALSE(DecodeFrameNotification("", h, &err));
    EXPECT_FALSE(DecodeFrameNotification("{not json}", h, &err));
    EXPECT_FALSE(DecodeFrameNotification(
        R"({"jsonrpc":"2.0","method":"frame","params":{"slot_index":)", h, &err));
    // Missing planes entirely.
    EXPECT_FALSE(DecodeFrameNotification(
        R"({"jsonrpc":"2.0","method":"frame","params":{"slot_index":0,"width":1,"height":1,"format":"NV12","plane_pitches":[],"plane_offsets":[],"frame_idx":0}})",
        h, &err));
}

TEST(DmabufMsg, RejectsWrongMethod) {
    Header h;
    std::string err;
    const char* wrong =
        R"({"jsonrpc":"2.0","method":"flush","params":{"slot_index":0,"width":1,"height":1,"format":"NV12","plane_pitches":[1],"plane_offsets":[0],"frame_idx":0}})";
    EXPECT_FALSE(DecodeFrameNotification(wrong, h, &err));
}

TEST(DmabufMsg, RejectsRequestWithId) {
    // A Request (has "id") is not a Notification — should be rejected.
    Header h;
    std::string err;
    const char* with_id =
        R"({"jsonrpc":"2.0","method":"frame","id":1,"params":{"slot_index":0,"width":1,"height":1,"format":"NV12","plane_pitches":[1],"plane_offsets":[0],"frame_idx":0}})";
    EXPECT_FALSE(DecodeFrameNotification(with_id, h, &err));
}

TEST(DmabufMsg, RejectsPitchOffsetMismatch) {
    Header h;
    std::string err;
    const char* bad =
        R"({"jsonrpc":"2.0","method":"frame","params":{"slot_index":0,"width":1920,"height":1080,"format":"NV12","plane_pitches":[1920,1920],"plane_offsets":[0],"frame_idx":0}})";
    EXPECT_FALSE(DecodeFrameNotification(bad, h, &err));
}

TEST(DmabufMsg, ForwardCompatUnknownKeysIgnored) {
    // Daemon may add fields we don't know about yet — they must be
    // skipped, not crash the parser.
    const char* fwd =
        R"({"jsonrpc":"2.0","method":"frame","params":{"slot_index":0,"width":640,"height":480,"format":"NV12","plane_pitches":[640],"plane_offsets":[0],"frame_idx":7,"future_field":"hello","another":[1,2,3],"obj":{"k":"v"}}})";
    Header h;
    std::string err;
    EXPECT_TRUE(DecodeFrameNotification(fwd, h, &err));
    EXPECT_EQ(h.width, 640u);
    EXPECT_EQ(h.frame_idx, 7u);
}

TEST(DmabufMsg, EscapesInFormatString) {
    // Defensive — format strings shouldn't have escapes, but the
    // parser should handle them anyway.
    const char* esc =
        R"({"jsonrpc":"2.0","method":"frame","params":{"slot_index":0,"width":1,"height":1,"format":"NV\/12","plane_pitches":[1],"plane_offsets":[0],"frame_idx":0}})";
    Header h;
    EXPECT_TRUE(DecodeFrameNotification(esc, h, nullptr));
    EXPECT_EQ(h.format, "NV/12");
}

TEST(DmabufMsg, LargeFrameIdxRoundtrip) {
    // frame_idx is uint64_t — verify the full range round-trips.
    Header h;
    h.width = 1;
    h.height = 1;
    h.format = "NV12";
    h.plane_pitches = {1};
    h.plane_offsets = {0};
    h.frame_idx = 18446744073709551615ULL; // UINT64_MAX
    std::string s = EncodeFrameNotification(h);
    Header back;
    EXPECT_TRUE(DecodeFrameNotification(s, back, nullptr));
    EXPECT_EQ(back.frame_idx, 18446744073709551615ULL);
}

TEST(DmabufMsg, NonzeroOffsetsMultiPlane) {
    // Real NV12 layout — chroma offset is width*height past Y plane.
    Header h;
    h.slot_index = 0;
    h.width = 1920;
    h.height = 1080;
    h.format = "NV12";
    h.plane_pitches = {1920, 1920};
    h.plane_offsets = {0, 1920 * 1080};
    h.frame_idx = 1;
    std::string s = EncodeFrameNotification(h);
    Header back;
    EXPECT_TRUE(DecodeFrameNotification(s, back, nullptr));
    EXPECT_EQ(back.plane_offsets[1], uint32_t(1920 * 1080));
}

TEST(DmabufMsg, NonNv12FormatRoundtrip) {
    // Future-proof: BGR3 / YUYV / etc. all just strings.
    for (const char* fmt : {"BGR3", "YUYV", "UYVY", "NV16", "NV24"}) {
        Header h;
        h.width = 16;
        h.height = 16;
        h.format = fmt;
        h.plane_pitches = {32};
        h.plane_offsets = {0};
        h.frame_idx = 1;
        std::string s = EncodeFrameNotification(h);
        Header back;
        EXPECT_TRUE(DecodeFrameNotification(s, back, nullptr));
        EXPECT_EQ(back.format, std::string(fmt));
    }
}

TEST(DmabufMsg, WhitespaceInParams) {
    // Producer might pretty-print for debugging — must still decode.
    const char* s = "{\n"
                    "  \"jsonrpc\": \"2.0\",\n"
                    "  \"method\": \"frame\",\n"
                    "  \"params\": {\n"
                    "    \"slot_index\": 0,\n"
                    "    \"width\": 16,\n"
                    "    \"height\": 16,\n"
                    "    \"format\": \"NV12\",\n"
                    "    \"plane_pitches\": [16],\n"
                    "    \"plane_offsets\": [0],\n"
                    "    \"frame_idx\": 1\n"
                    "  }\n"
                    "}";
    Header h;
    std::string err;
    EXPECT_TRUE(DecodeFrameNotification(s, h, &err));
    EXPECT_EQ(h.width, 16u);
}

TEST(DmabufMsg, RejectsMissingParams) {
    const char* no_params = R"({"jsonrpc":"2.0","method":"frame"})";
    Header h;
    std::string err;
    EXPECT_FALSE(DecodeFrameNotification(no_params, h, &err));
}

TEST(DmabufMsg, RejectsRequestKindViaIdPresence) {
    // Even if method == "frame", presence of id makes it a Request.
    const char* req =
        R"({"jsonrpc":"2.0","method":"frame","id":1,"params":{"slot_index":0,"width":1,"height":1,"format":"NV12","plane_pitches":[1],"plane_offsets":[0],"frame_idx":0}})";
    Header h;
    std::string err;
    EXPECT_FALSE(DecodeFrameNotification(req, h, &err));
}
