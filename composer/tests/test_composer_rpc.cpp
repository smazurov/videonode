// Tests for composer_rpc params parsers.

#include "src/rpc/composer_rpc.hpp"

#include <gtest/gtest.h>

using composer_rpc::ParseError;

// ----- set_canvas -----------------------------------------------------

TEST(SetCanvas, HappyPath) {
    composer_rpc::SetCanvasRequest r;
    ParseError e;
    ASSERT_TRUE(parse_set_canvas(R"({"w":1920,"h":1080,"fps":30})", r, e));
    EXPECT_EQ(1920u, r.w);
    EXPECT_EQ(1080u, r.h);
    EXPECT_EQ(30u, r.fps);
}

TEST(SetCanvas, MissingField) {
    composer_rpc::SetCanvasRequest r;
    ParseError e;
    EXPECT_FALSE(parse_set_canvas(R"({"w":1920,"h":1080})", r, e));
    EXPECT_EQ(-32602, e.code);
}

TEST(SetCanvas, OutOfRange) {
    composer_rpc::SetCanvasRequest r;
    ParseError e;
    EXPECT_FALSE(parse_set_canvas(R"({"w":99999,"h":1080,"fps":30})", r, e));
    EXPECT_FALSE(parse_set_canvas(R"({"w":1920,"h":1080,"fps":999})", r, e));
}

TEST(SetCanvas, NotObject) {
    composer_rpc::SetCanvasRequest r;
    ParseError e;
    EXPECT_FALSE(parse_set_canvas("[]", r, e));
}

TEST(SetCanvas, IgnoresUnknownKey) {
    composer_rpc::SetCanvasRequest r;
    ParseError e;
    EXPECT_TRUE(parse_set_canvas(R"({"w":1280,"h":720,"fps":60,"extra":"ok"})", r, e));
    EXPECT_EQ(1280u, r.w);
}

// ----- set_source -----------------------------------------------------

TEST(SetSource, HappyPath) {
    composer_rpc::SetSourceRequest r;
    ParseError e;
    ASSERT_TRUE(parse_set_source(
        R"({"slot":"a","source_id":"hdmi-1","scm_path":"/tmp/s.sock","width":3840,"height":2160,"fps":60})",
        r, e));
    EXPECT_EQ("a", r.slot);
    EXPECT_EQ("hdmi-1", r.source_id);
    EXPECT_EQ("/tmp/s.sock", r.scm_path);
    EXPECT_EQ(3840u, r.width);
    EXPECT_EQ(2160u, r.height);
    EXPECT_EQ(60u, r.fps);
}

TEST(SetSource, EmptyStringRejected) {
    composer_rpc::SetSourceRequest r;
    ParseError e;
    EXPECT_FALSE(parse_set_source(
        R"({"slot":"","source_id":"hdmi-1","scm_path":"/tmp/s.sock","width":1920,"height":1080,"fps":30})",
        r, e));
}

TEST(SetSource, MissingField) {
    composer_rpc::SetSourceRequest r;
    ParseError e;
    EXPECT_FALSE(parse_set_source(
        R"({"slot":"a","source_id":"hdmi-1","width":1920,"height":1080,"fps":30})",
        r, e));
}

// ----- clear_source ---------------------------------------------------

TEST(ClearSource, HappyPath) {
    composer_rpc::ClearSourceRequest r;
    ParseError e;
    ASSERT_TRUE(parse_clear_source(R"({"slot":"b"})", r, e));
    EXPECT_EQ("b", r.slot);
}

TEST(ClearSource, MissingSlot) {
    composer_rpc::ClearSourceRequest r;
    ParseError e;
    EXPECT_FALSE(parse_clear_source(R"({})", r, e));
}

// ----- set_layout -----------------------------------------------------

TEST(SetLayout, HappyPath) {
    composer_rpc::SetLayoutRequest r;
    ParseError e;
    ASSERT_TRUE(parse_set_layout(
        R"({"slots":[{"slot":"a","x":0,"y":0,"w":960,"h":1080},{"slot":"b","x":960,"y":0,"w":960,"h":1080}]})",
        r, e));
    ASSERT_EQ(2u, r.slots.size());
    EXPECT_EQ("a", r.slots[0].slot);
    EXPECT_EQ(0, r.slots[0].x);
    EXPECT_EQ(960, r.slots[0].w);
    EXPECT_EQ("b", r.slots[1].slot);
    EXPECT_EQ(960, r.slots[1].x);
}

TEST(SetLayout, EmptySlotsArray) {
    composer_rpc::SetLayoutRequest r;
    ParseError e;
    EXPECT_TRUE(parse_set_layout(R"({"slots":[]})", r, e));
    EXPECT_EQ(0u, r.slots.size());
}

TEST(SetLayout, NonPositiveDims) {
    composer_rpc::SetLayoutRequest r;
    ParseError e;
    EXPECT_FALSE(parse_set_layout(
        R"({"slots":[{"slot":"a","x":0,"y":0,"w":0,"h":1080}]})", r, e));
}

TEST(SetLayout, NegativeOffsetOk) {
    // x/y signed — negative is valid (off-canvas slots).
    composer_rpc::SetLayoutRequest r;
    ParseError e;
    ASSERT_TRUE(parse_set_layout(
        R"({"slots":[{"slot":"a","x":-50,"y":-100,"w":960,"h":1080}]})", r, e));
    EXPECT_EQ(-50, r.slots[0].x);
    EXPECT_EQ(-100, r.slots[0].y);
}

// ----- set_effects ----------------------------------------------------

TEST(SetEffects, PerspectiveHappyPath) {
    composer_rpc::SetEffectsRequest r;
    ParseError e;
    ASSERT_TRUE(parse_set_effects(
        R"({"source_id":"hdmi-1","effects":[{"type":"perspective","corners":[[0,0],[1919,0],[1919,1079],[0,1079]],"snapshot_w":1920,"snapshot_h":1080}]})",
        r, e));
    EXPECT_EQ("hdmi-1", r.source_id);
    ASSERT_EQ(1u, r.effects.size());
    EXPECT_EQ("perspective", r.effects[0].type);
    EXPECT_TRUE(r.effects[0].recognized);
    ASSERT_TRUE(r.effects[0].perspective.has_value());
    const auto& p = *r.effects[0].perspective;
    EXPECT_EQ(0, p.corners[0]);
    EXPECT_EQ(0, p.corners[1]);
    EXPECT_EQ(1919, p.corners[2]);
    EXPECT_EQ(1079, p.corners[5]);
    EXPECT_EQ(0, p.corners[6]);
    EXPECT_EQ(1920, p.snapshot_w);
    EXPECT_EQ(1080, p.snapshot_h);
}

TEST(SetEffects, EmptyEffectsArrayClearsEffects) {
    composer_rpc::SetEffectsRequest r;
    ParseError e;
    ASSERT_TRUE(parse_set_effects(R"({"source_id":"hdmi-1","effects":[]})", r, e));
    EXPECT_EQ(0u, r.effects.size());
}

TEST(SetEffects, UnknownTypeMarkedUnrecognized) {
    // crop/bbox aren't implemented yet — parser still accepts the
    // message so future composer versions can act on it without daemon
    // changes; recognized=false signals "log + skip".
    composer_rpc::SetEffectsRequest r;
    ParseError e;
    ASSERT_TRUE(parse_set_effects(
        R"({"source_id":"hdmi-1","effects":[{"type":"crop","rect":[0,0,100,100]}]})",
        r, e));
    ASSERT_EQ(1u, r.effects.size());
    EXPECT_EQ("crop", r.effects[0].type);
    EXPECT_FALSE(r.effects[0].recognized);
    EXPECT_FALSE(r.effects[0].perspective.has_value());
}

TEST(SetEffects, PerspectiveMissingSnapshot) {
    composer_rpc::SetEffectsRequest r;
    ParseError e;
    EXPECT_FALSE(parse_set_effects(
        R"({"source_id":"hdmi-1","effects":[{"type":"perspective","corners":[[0,0],[1,0],[1,1],[0,1]]}]})",
        r, e));
}

TEST(SetEffects, PerspectiveNonPositiveSnapshot) {
    composer_rpc::SetEffectsRequest r;
    ParseError e;
    EXPECT_FALSE(parse_set_effects(
        R"({"source_id":"hdmi-1","effects":[{"type":"perspective","corners":[[0,0],[1,0],[1,1],[0,1]],"snapshot_w":0,"snapshot_h":1080}]})",
        r, e));
}

TEST(SetEffects, FlatCornersAlsoAccepted) {
    // Tolerate both [[x,y],...] (API model shape) and [x,y,x,y,...] flat.
    composer_rpc::SetEffectsRequest r;
    ParseError e;
    ASSERT_TRUE(parse_set_effects(
        R"({"source_id":"x","effects":[{"type":"perspective","corners":[0,0,1919,0,1919,1079,0,1079],"snapshot_w":1920,"snapshot_h":1080}]})",
        r, e));
    ASSERT_TRUE(r.effects[0].perspective.has_value());
    EXPECT_EQ(1919, r.effects[0].perspective->corners[2]);
}

TEST(SetEffects, MultipleEffectsOrderPreserved) {
    composer_rpc::SetEffectsRequest r;
    ParseError e;
    ASSERT_TRUE(parse_set_effects(
        R"({"source_id":"x","effects":[{"type":"crop"},{"type":"perspective","corners":[[0,0],[1,0],[1,1],[0,1]],"snapshot_w":2,"snapshot_h":2},{"type":"bbox"}]})",
        r, e));
    ASSERT_EQ(3u, r.effects.size());
    EXPECT_EQ("crop", r.effects[0].type);
    EXPECT_EQ("perspective", r.effects[1].type);
    EXPECT_EQ("bbox", r.effects[2].type);
}

// ----- set_source_state -----------------------------------------------

TEST(SetSourceState, HappyPath) {
    composer_rpc::SetSourceStateRequest r;
    ParseError e;
    ASSERT_TRUE(parse_set_source_state(R"({"source_id":"hdmi-1","state":"live"})", r, e));
    EXPECT_EQ("hdmi-1", r.source_id);
    EXPECT_EQ("live", r.state);
}

TEST(SetSourceState, AcceptsAllThreeValues) {
    for (const char* v : {"live", "transitioning", "placeholder"}) {
        composer_rpc::SetSourceStateRequest r;
        ParseError e;
        std::string body = std::string(R"({"source_id":"hdmi-1","state":")") + v + R"("})";
        ASSERT_TRUE(parse_set_source_state(body, r, e)) << v;
        EXPECT_EQ(v, r.state);
    }
}

TEST(SetSourceState, RejectsUnknownState) {
    composer_rpc::SetSourceStateRequest r;
    ParseError e;
    EXPECT_FALSE(parse_set_source_state(R"({"source_id":"hdmi-1","state":"frobnicating"})", r, e));
}

TEST(SetSourceState, MissingFields) {
    composer_rpc::SetSourceStateRequest r;
    ParseError e;
    EXPECT_FALSE(parse_set_source_state(R"({"source_id":"hdmi-1"})", r, e));
    EXPECT_FALSE(parse_set_source_state(R"({"state":"live"})", r, e));
}
