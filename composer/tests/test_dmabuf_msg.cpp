#include "../src/dmabuf_msg.hpp"
#include "test_runner.hpp"

int main() {
    using namespace dmabuf_msg;

    test_runner::start_case("encode_decode_roundtrip_single_plane");
    {
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
        CHECK_TRUE(s.find("\"jsonrpc\":\"2.0\"") != std::string::npos);
        CHECK_TRUE(s.find("\"method\":\"frame\"") != std::string::npos);
        // No id (it's a notification).
        CHECK_TRUE(s.find("\"id\"") == std::string::npos);
        // Inner params sentinels.
        CHECK_TRUE(s.find("\"slot_index\":1") != std::string::npos);
        CHECK_TRUE(s.find("\"format\":\"NV12\"") != std::string::npos);
        CHECK_TRUE(s.find("\"plane_pitches\":[1920]") != std::string::npos);
        CHECK_TRUE(s.find("\"color_matrix\":1") != std::string::npos);
        CHECK_TRUE(s.find("\"color_range\":1") != std::string::npos);
        CHECK_TRUE(s.find("\"chroma_siting\":1") != std::string::npos);

        Header back;
        std::string err;
        CHECK_TRUE(DecodeFrameNotification(s, back, &err));
        CHECK_EQ(back.slot_index, 1u);
        CHECK_EQ(back.width, 1920u);
        CHECK_EQ(back.height, 1080u);
        CHECK_STR_EQ(back.format, "NV12");
        CHECK_EQ(back.plane_pitches.size(), 1u);
        CHECK_EQ(back.plane_pitches[0], 1920u);
        CHECK_EQ(back.plane_offsets.size(), 1u);
        CHECK_EQ(back.plane_offsets[0], 0u);
        CHECK_TRUE(back.color_matrix == ColorMatrix::Bt601);
        CHECK_TRUE(back.color_range == ColorRange::Limited);
        CHECK_TRUE(back.chroma_siting == ChromaSiting::Mpeg2);
        CHECK_EQ(back.frame_idx, 42u);
    }

    test_runner::start_case("decode_missing_color_metadata_defaults_to_unspecified");
    {
        // Forward-compat: older producers (or hand-rolled tests) may
        // omit the color metadata fields. Decoder must accept and leave
        // them at Unspecified so the consumer can apply its fallback.
        const char* old_style =
            R"({"jsonrpc":"2.0","method":"frame","params":{"slot_index":0,"width":640,"height":480,"format":"NV12","plane_pitches":[640],"plane_offsets":[0],"frame_idx":1}})";
        Header h;
        std::string err;
        CHECK_TRUE(DecodeFrameNotification(old_style, h, &err));
        CHECK_TRUE(h.color_matrix == ColorMatrix::Unspecified);
        CHECK_TRUE(h.color_range == ColorRange::Unspecified);
        CHECK_TRUE(h.chroma_siting == ChromaSiting::Unspecified);
    }

    test_runner::start_case("encode_decode_roundtrip_multi_plane");
    {
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
        CHECK_TRUE(DecodeFrameNotification(s, back, nullptr));
        CHECK_EQ(back.plane_pitches.size(), 2u);
        CHECK_EQ(back.plane_pitches[0], 1280u);
        CHECK_EQ(back.plane_pitches[1], 1280u);
        CHECK_EQ(back.plane_offsets.size(), 2u);
    }

    test_runner::start_case("decodes_go_produced_envelope");
    {
        // Sample JSON-RPC notification shaped exactly like what Go's
        // encoding/json would produce from the equivalent struct on the
        // daemon side.
        const char* go =
            R"({"jsonrpc":"2.0","method":"frame","params":{"slot_index":2,"width":3840,"height":2160,"format":"NV12","plane_pitches":[3840],"plane_offsets":[0],"frame_idx":99}})";
        Header back;
        std::string err;
        CHECK_TRUE(DecodeFrameNotification(go, back, &err));
        CHECK_EQ(back.slot_index, 2u);
        CHECK_EQ(back.width, 3840u);
        CHECK_EQ(back.height, 2160u);
        CHECK_STR_EQ(back.format, "NV12");
        CHECK_EQ(back.frame_idx, 99u);
    }

    test_runner::start_case("rejects_malformed");
    {
        Header h;
        std::string err;
        CHECK_TRUE(!DecodeFrameNotification("", h, &err));
        CHECK_TRUE(!DecodeFrameNotification("{not json}", h, &err));
        CHECK_TRUE(!DecodeFrameNotification(R"({"jsonrpc":"2.0","method":"frame","params":{"slot_index":)", h, &err));
        // Missing planes entirely.
        CHECK_TRUE(!DecodeFrameNotification(
            R"({"jsonrpc":"2.0","method":"frame","params":{"slot_index":0,"width":1,"height":1,"format":"NV12","plane_pitches":[],"plane_offsets":[],"frame_idx":0}})",
            h, &err));
    }

    test_runner::start_case("rejects_wrong_method");
    {
        Header h;
        std::string err;
        const char* wrong =
            R"({"jsonrpc":"2.0","method":"flush","params":{"slot_index":0,"width":1,"height":1,"format":"NV12","plane_pitches":[1],"plane_offsets":[0],"frame_idx":0}})";
        CHECK_TRUE(!DecodeFrameNotification(wrong, h, &err));
    }

    test_runner::start_case("rejects_request_with_id");
    {
        // A Request (has "id") is not a Notification — should be rejected.
        Header h;
        std::string err;
        const char* with_id =
            R"({"jsonrpc":"2.0","method":"frame","id":1,"params":{"slot_index":0,"width":1,"height":1,"format":"NV12","plane_pitches":[1],"plane_offsets":[0],"frame_idx":0}})";
        CHECK_TRUE(!DecodeFrameNotification(with_id, h, &err));
    }

    test_runner::start_case("rejects_pitch_offset_mismatch");
    {
        Header h;
        std::string err;
        const char* bad =
            R"({"jsonrpc":"2.0","method":"frame","params":{"slot_index":0,"width":1920,"height":1080,"format":"NV12","plane_pitches":[1920,1920],"plane_offsets":[0],"frame_idx":0}})";
        CHECK_TRUE(!DecodeFrameNotification(bad, h, &err));
    }

    test_runner::start_case("forward_compat_unknown_keys_ignored");
    {
        // Daemon may add fields we don't know about yet — they must be
        // skipped, not crash the parser.
        const char* fwd =
            R"({"jsonrpc":"2.0","method":"frame","params":{"slot_index":0,"width":640,"height":480,"format":"NV12","plane_pitches":[640],"plane_offsets":[0],"frame_idx":7,"future_field":"hello","another":[1,2,3],"obj":{"k":"v"}}})";
        Header h;
        std::string err;
        CHECK_TRUE(DecodeFrameNotification(fwd, h, &err));
        CHECK_EQ(h.width, 640u);
        CHECK_EQ(h.frame_idx, 7u);
    }

    test_runner::start_case("escapes_in_format_string");
    {
        // Defensive — format strings shouldn't have escapes, but the
        // parser should handle them anyway.
        const char* esc =
            R"({"jsonrpc":"2.0","method":"frame","params":{"slot_index":0,"width":1,"height":1,"format":"NV\/12","plane_pitches":[1],"plane_offsets":[0],"frame_idx":0}})";
        Header h;
        CHECK_TRUE(DecodeFrameNotification(esc, h, nullptr));
        CHECK_STR_EQ(h.format, "NV/12");
    }

    test_runner::start_case("large_frame_idx_roundtrip");
    {
        // frame_idx is uint64_t — verify the full range round-trips.
        Header h;
        h.width = 1; h.height = 1; h.format = "NV12";
        h.plane_pitches = {1}; h.plane_offsets = {0};
        h.frame_idx = 18446744073709551615ULL; // UINT64_MAX
        std::string s = EncodeFrameNotification(h);
        Header back;
        CHECK_TRUE(DecodeFrameNotification(s, back, nullptr));
        CHECK_EQ(back.frame_idx, 18446744073709551615ULL);
    }

    test_runner::start_case("nonzero_offsets_multi_plane");
    {
        // Real NV12 layout — chroma offset is width*height past Y plane.
        Header h;
        h.slot_index = 0;
        h.width = 1920; h.height = 1080; h.format = "NV12";
        h.plane_pitches = {1920, 1920};
        h.plane_offsets = {0, 1920 * 1080};
        h.frame_idx = 1;
        std::string s = EncodeFrameNotification(h);
        Header back;
        CHECK_TRUE(DecodeFrameNotification(s, back, nullptr));
        CHECK_EQ(back.plane_offsets[1], uint32_t(1920 * 1080));
    }

    test_runner::start_case("non_nv12_format_roundtrip");
    {
        // Future-proof: BGR3 / YUYV / etc. all just strings.
        for (const char* fmt : {"BGR3", "YUYV", "UYVY", "NV16", "NV24"}) {
            Header h;
            h.width = 16; h.height = 16; h.format = fmt;
            h.plane_pitches = {32}; h.plane_offsets = {0};
            h.frame_idx = 1;
            std::string s = EncodeFrameNotification(h);
            Header back;
            CHECK_TRUE(DecodeFrameNotification(s, back, nullptr));
            CHECK_STR_EQ(back.format, fmt);
        }
    }

    test_runner::start_case("whitespace_in_params");
    {
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
        CHECK_TRUE(DecodeFrameNotification(s, h, &err));
        CHECK_EQ(h.width, 16u);
    }

    test_runner::start_case("rejects_missing_params");
    {
        const char* no_params = R"({"jsonrpc":"2.0","method":"frame"})";
        Header h;
        std::string err;
        CHECK_TRUE(!DecodeFrameNotification(no_params, h, &err));
    }

    test_runner::start_case("rejects_request_kind_via_id_presence");
    {
        // Even if method == "frame", presence of id makes it a Request.
        const char* req =
            R"({"jsonrpc":"2.0","method":"frame","id":1,"params":{"slot_index":0,"width":1,"height":1,"format":"NV12","plane_pitches":[1],"plane_offsets":[0],"frame_idx":0}})";
        Header h;
        std::string err;
        CHECK_TRUE(!DecodeFrameNotification(req, h, &err));
    }

    return test_runner::report_and_exit_code();
}
