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
        h.frame_idx = 42;

        std::string s = EncodeHeader(h);
        // Spot-check some sentinels in the JSON.
        CHECK_TRUE(s.find("\"slot_index\":1") != std::string::npos);
        CHECK_TRUE(s.find("\"format\":\"NV12\"") != std::string::npos);
        CHECK_TRUE(s.find("\"plane_pitches\":[1920]") != std::string::npos);

        Header back;
        std::string err;
        CHECK_TRUE(DecodeHeader(s, back, &err));
        CHECK_EQ(back.slot_index, 1u);
        CHECK_EQ(back.width, 1920u);
        CHECK_EQ(back.height, 1080u);
        CHECK_STR_EQ(back.format, "NV12");
        CHECK_EQ(back.plane_pitches.size(), 1u);
        CHECK_EQ(back.plane_pitches[0], 1920u);
        CHECK_EQ(back.plane_offsets.size(), 1u);
        CHECK_EQ(back.plane_offsets[0], 0u);
        CHECK_EQ(back.frame_idx, 42u);
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
        std::string s = EncodeHeader(h);

        Header back;
        CHECK_TRUE(DecodeHeader(s, back, nullptr));
        CHECK_EQ(back.plane_pitches.size(), 2u);
        CHECK_EQ(back.plane_pitches[0], 1280u);
        CHECK_EQ(back.plane_pitches[1], 1280u);
        CHECK_EQ(back.plane_offsets.size(), 2u);
    }

    test_runner::start_case("decodes_go_produced_json");
    {
        // Sample JSON shaped exactly like what Go's encoding/json would
        // produce from the equivalent struct on the daemon side.
        const char* go =
            R"({"slot_index":2,"width":3840,"height":2160,"format":"NV12","plane_pitches":[3840],"plane_offsets":[0],"frame_idx":99})";
        Header back;
        std::string err;
        CHECK_TRUE(DecodeHeader(go, back, &err));
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
        CHECK_TRUE(!DecodeHeader("", h, &err));
        CHECK_TRUE(!DecodeHeader("{not json}", h, &err));
        CHECK_TRUE(!DecodeHeader(R"({"slot_index":)", h, &err));
        // Missing planes entirely.
        CHECK_TRUE(!DecodeHeader(
            R"({"slot_index":0,"width":1,"height":1,"format":"NV12","plane_pitches":[],"plane_offsets":[],"frame_idx":0})",
            h, &err));
    }

    test_runner::start_case("rejects_pitch_offset_mismatch");
    {
        Header h;
        std::string err;
        const char* bad =
            R"({"slot_index":0,"width":1920,"height":1080,"format":"NV12","plane_pitches":[1920,1920],"plane_offsets":[0],"frame_idx":0})";
        CHECK_TRUE(!DecodeHeader(bad, h, &err));
    }

    test_runner::start_case("forward_compat_unknown_keys_ignored");
    {
        // Daemon may add fields we don't know about yet — they must be
        // skipped, not crash the parser.
        const char* fwd =
            R"({"slot_index":0,"width":640,"height":480,"format":"NV12","plane_pitches":[640],"plane_offsets":[0],"frame_idx":7,"future_field":"hello","another":[1,2,3],"obj":{"k":"v"}})";
        Header h;
        std::string err;
        CHECK_TRUE(DecodeHeader(fwd, h, &err));
        CHECK_EQ(h.width, 640u);
        CHECK_EQ(h.frame_idx, 7u);
    }

    test_runner::start_case("escapes_in_format_string");
    {
        // Defensive — format strings shouldn't have escapes, but the
        // parser should handle them anyway.
        const char* esc =
            R"({"slot_index":0,"width":1,"height":1,"format":"NV\/12","plane_pitches":[1],"plane_offsets":[0],"frame_idx":0})";
        Header h;
        CHECK_TRUE(DecodeHeader(esc, h, nullptr));
        CHECK_STR_EQ(h.format, "NV/12");
    }

    return test_runner::report_and_exit_code();
}
