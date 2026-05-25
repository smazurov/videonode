// Tests for render::World — mutex-protected daemon-pushed state mirror.

#include "src/render/world.hpp"

#include <gtest/gtest.h>

#include <atomic>
#include <span>
#include <thread>

namespace {

composer_rpc::SetCanvasRequest canvas(uint32_t w, uint32_t h, uint32_t fps) {
    composer_rpc::SetCanvasRequest r;
    r.w = w;
    r.h = h;
    r.fps = fps;
    return r;
}

composer_rpc::SetSourceRequest source(const char* slot, const char* sid, const char* path,
                                      uint32_t w = 1920, uint32_t h = 1080, uint32_t fps = 30) {
    composer_rpc::SetSourceRequest r;
    r.slot = slot;
    r.source_id = sid;
    r.scm_path = path;
    r.width = w;
    r.height = h;
    r.fps = fps;
    return r;
}

composer_rpc::SetLayoutRequest layout_one(const char* slot, int32_t x, int32_t y, int32_t w,
                                          int32_t h) {
    composer_rpc::SetLayoutRequest r;
    composer_rpc::LayoutSlot ls;
    ls.slot = slot;
    ls.x = x;
    ls.y = y;
    ls.w = w;
    ls.h = h;
    r.slots.push_back(std::move(ls));
    return r;
}

composer_rpc::SetEffectsRequest perspective_effect(const char* sid, std::span<const int, 8> corners,
                                                   int snap_w, int snap_h) {
    composer_rpc::SetEffectsRequest r;
    r.source_id = sid;
    composer_rpc::Effect e;
    e.type = "perspective";
    e.recognized = true;
    composer_rpc::PerspectiveEffectParams p{};
    std::span<int32_t, 8> dst(p.corners);
    for (int i = 0; i < 8; ++i)
        dst[i] = corners[i];
    p.snapshot_w = snap_w;
    p.snapshot_h = snap_h;
    e.perspective = p;
    r.effects.push_back(std::move(e));
    return r;
}

} // namespace

TEST(World, StartsNotReady) {
    render::World w;
    auto s = w.snapshot();
    EXPECT_FALSE(s.ready);
    EXPECT_EQ(0u, s.canvas_w);
    EXPECT_TRUE(s.slots.empty());
}

TEST(World, NotReadyWithCanvasAlone) {
    render::World w;
    composer_rpc::ParseError e;
    ASSERT_TRUE(w.apply_set_canvas(canvas(1920, 1080, 30), e));
    auto s = w.snapshot();
    EXPECT_FALSE(s.ready) << "canvas without bound+placed slot must remain not-ready";
    EXPECT_EQ(1920u, s.canvas_w);
}

TEST(World, NotReadyWithSourceButNoLayout) {
    render::World w;
    composer_rpc::ParseError e;
    ASSERT_TRUE(w.apply_set_canvas(canvas(1920, 1080, 30), e));
    ASSERT_TRUE(w.apply_set_source(source("a", "hdmi-1", "/tmp/s.sock"), e));
    auto s = w.snapshot();
    EXPECT_FALSE(s.ready) << "bound slot without layout entry must remain not-ready";
}

TEST(World, ReadyAfterCanvasSourceLayout) {
    render::World w;
    composer_rpc::ParseError e;
    ASSERT_TRUE(w.apply_set_canvas(canvas(1920, 1080, 30), e));
    ASSERT_TRUE(w.apply_set_source(source("a", "hdmi-1", "/tmp/s.sock"), e));
    ASSERT_TRUE(w.apply_set_layout(layout_one("a", 0, 0, 1920, 1080), e));
    auto s = w.snapshot();
    EXPECT_TRUE(s.ready);
    ASSERT_EQ(1u, s.slots.size());
    EXPECT_EQ("a", s.slots[0].slot);
    EXPECT_EQ("hdmi-1", s.slots[0].source_id);
    ASSERT_EQ(1u, s.layout.size());
    EXPECT_EQ(1920, s.layout[0].w);
}

TEST(World, ClearSourceUnsetsSlot) {
    render::World w;
    composer_rpc::ParseError e;
    ASSERT_TRUE(w.apply_set_source(source("a", "hdmi-1", "/tmp/s.sock"), e));
    composer_rpc::ClearSourceRequest c;
    c.slot = "a";
    ASSERT_TRUE(w.apply_clear_source(c, e));
    auto s = w.snapshot();
    EXPECT_TRUE(s.slots.empty());
}

TEST(World, ClearSourceUnknownSlotErrors) {
    render::World w;
    composer_rpc::ParseError e;
    composer_rpc::ClearSourceRequest c;
    c.slot = "z";
    EXPECT_FALSE(w.apply_clear_source(c, e));
    EXPECT_NE(0, e.code);
}

TEST(World, SetLayoutReplacesEntirely) {
    render::World w;
    composer_rpc::ParseError e;
    ASSERT_TRUE(w.apply_set_layout(layout_one("a", 0, 0, 960, 1080), e));
    auto s1 = w.snapshot();
    ASSERT_EQ(1u, s1.layout.size());
    EXPECT_EQ(960, s1.layout[0].w);

    composer_rpc::SetLayoutRequest l2;
    composer_rpc::LayoutSlot a, b;
    a.slot = "a";
    a.x = 0;
    a.y = 0;
    a.w = 480;
    a.h = 1080;
    b.slot = "b";
    b.x = 480;
    b.y = 0;
    b.w = 1440;
    b.h = 1080;
    l2.slots = {a, b};
    ASSERT_TRUE(w.apply_set_layout(l2, e));
    auto s2 = w.snapshot();
    EXPECT_EQ(2u, s2.layout.size());
}

TEST(World, SetEffectsSolvesPerspective) {
    render::World w;
    composer_rpc::ParseError e;
    int corners[8] = {96, 0, 1823, 0, 1919, 1079, 0, 1079};
    ASSERT_TRUE(w.apply_set_effects(perspective_effect("hdmi-1", corners, 1920, 1080), e));
    auto s = w.snapshot();
    auto it = s.source_states.find("hdmi-1");
    ASSERT_NE(s.source_states.end(), it);
    EXPECT_TRUE(it->second.has_perspective);
    // warp is not identity if perspective was solved
    bool any_off_diagonal = (it->second.warp[1] != 0.0f || it->second.warp[3] != 0.0f ||
                             it->second.warp[6] != 0.0f || it->second.warp[7] != 0.0f);
    EXPECT_TRUE(any_off_diagonal);
}

TEST(World, SetEffectsEmptyResetsToIdentity) {
    render::World w;
    composer_rpc::ParseError e;
    int corners[8] = {96, 0, 1823, 0, 1919, 1079, 0, 1079};
    ASSERT_TRUE(w.apply_set_effects(perspective_effect("hdmi-1", corners, 1920, 1080), e));

    composer_rpc::SetEffectsRequest empty;
    empty.source_id = "hdmi-1";
    ASSERT_TRUE(w.apply_set_effects(empty, e));

    auto s = w.snapshot();
    auto it = s.source_states.find("hdmi-1");
    ASSERT_NE(s.source_states.end(), it);
    EXPECT_FALSE(it->second.has_perspective);
    // Identity warp
    EXPECT_EQ(1.0f, it->second.warp[0]);
    EXPECT_EQ(0.0f, it->second.warp[1]);
    EXPECT_EQ(1.0f, it->second.warp[4]);
    EXPECT_EQ(1.0f, it->second.warp[8]);
}

TEST(World, SetEffectsRejectsDegenerate) {
    render::World w;
    composer_rpc::ParseError e;
    int corners[8] = {0, 0, 0, 0, 1919, 1079, 0, 1079}; // two coincident
    EXPECT_FALSE(w.apply_set_effects(perspective_effect("hdmi-1", corners, 1920, 1080), e));
    EXPECT_NE(0, e.code);
}

TEST(World, SetSourceStateUpdatesPerSource) {
    render::World w;
    composer_rpc::ParseError e;
    composer_rpc::SetSourceStateRequest r;
    r.source_id = "hdmi-1";
    r.state = "placeholder";
    ASSERT_TRUE(w.apply_set_source_state(r, e));
    auto s = w.snapshot();
    EXPECT_EQ("placeholder", s.source_states["hdmi-1"].state);

    r.state = "live";
    ASSERT_TRUE(w.apply_set_source_state(r, e));
    s = w.snapshot();
    EXPECT_EQ("live", s.source_states["hdmi-1"].state);
}

TEST(World, EffectsAndStateCanArriveInEitherOrder) {
    render::World w;
    composer_rpc::ParseError e;
    // State first, then effects.
    composer_rpc::SetSourceStateRequest sr;
    sr.source_id = "x";
    sr.state = "transitioning";
    ASSERT_TRUE(w.apply_set_source_state(sr, e));
    int corners[8] = {0, 0, 1919, 0, 1919, 1079, 0, 1079};
    ASSERT_TRUE(w.apply_set_effects(perspective_effect("x", corners, 1920, 1080), e));
    auto s = w.snapshot();
    EXPECT_EQ("transitioning", s.source_states["x"].state);
    EXPECT_TRUE(s.source_states["x"].has_perspective);
}

// Concurrency smoke. Writer thread mutates World repeatedly while the
// reader thread takes snapshots. No assertion about specific values
// (snapshots can race the writes); the test just exercises the locking
// path. TSan preset will catch any actual data race here.
TEST(World, ConcurrentReadWriteIsSafe) {
    render::World w;
    std::atomic<bool> stop{false};

    std::thread writer([&] {
        composer_rpc::ParseError e;
        int n = 0;
        while (!stop.load()) {
            (void)w.apply_set_canvas(canvas(1920, 1080, 30), e);
            (void)w.apply_set_source(source("a", "hdmi-1", "/tmp/s.sock"), e);
            (void)w.apply_set_layout(layout_one("a", 0, 0, 1920, 1080), e);
            composer_rpc::SetSourceStateRequest sr;
            sr.source_id = "hdmi-1";
            sr.state = (n++ % 2) ? "live" : "transitioning";
            (void)w.apply_set_source_state(sr, e);
        }
    });

    std::thread reader([&] {
        int seen = 0;
        while (seen < 1000) {
            auto s = w.snapshot();
            // Any consistent observation is fine; just don't crash.
            if (s.ready)
                ++seen;
        }
        stop.store(true);
    });

    writer.join();
    reader.join();
    EXPECT_TRUE(w.snapshot().ready);
}
