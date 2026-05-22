// Tests for the corners→3×3 homography solver.

#include "src/render/homography.hpp"

#include <gtest/gtest.h>

#include <array>
#include <cmath>

namespace {

constexpr float kEps = 1e-4f;

// Apply a 3x3 row-major homography to a UV point, returning the
// homogeneous-divided result.
std::array<float, 2> apply(const float h[9], float u, float v) {
    float x = h[0] * u + h[1] * v + h[2];
    float y = h[3] * u + h[4] * v + h[5];
    float w = h[6] * u + h[7] * v + h[8];
    return {x / w, y / w};
}

bool nearly_equal(float a, float b, float eps = kEps) {
    return std::fabs(a - b) <= eps;
}

} // namespace

TEST(Homography, IdentityCornersGiveIdentityMatrix) {
    // Source corners = full source rect → maps unit-square dest UVs to
    // unit-square source UVs (identity sampling).
    int corners[8] = {
        0, 0,
        1919, 0,
        1919, 1079,
        0, 1079,
    };
    float h[9] = {};
    auto s = homography::corners_to_warp(corners, 1920, 1080, h);
    ASSERT_EQ(homography::Status::Ok, s);

    // After homogeneous divide, each dest corner should map to the
    // matching source UV (which IS the dest UV here, modulo a small
    // 1/1920 quantization from the integer corner inputs).
    auto p00 = apply(h, 0.0f, 0.0f);
    EXPECT_TRUE(nearly_equal(p00[0], 0.0f, 1e-3f)) << "TL u=" << p00[0];
    EXPECT_TRUE(nearly_equal(p00[1], 0.0f, 1e-3f)) << "TL v=" << p00[1];

    auto p11 = apply(h, 1.0f, 1.0f);
    EXPECT_TRUE(nearly_equal(p11[0], 1919.0f / 1920.0f, 1e-3f)) << "BR u=" << p11[0];
    EXPECT_TRUE(nearly_equal(p11[1], 1079.0f / 1080.0f, 1e-3f)) << "BR v=" << p11[1];
}

TEST(Homography, KnownKeystoneTopInset) {
    // Same keystone the old hardcoded matrix encoded: top edge pulled
    // inward. With snapshot 1920×1080 and the top corners moved in by 5%
    // (~96 px), the bottom corners stay at the source rect's bottom
    // corners. Expect a non-identity matrix that, when applied to the
    // top-middle dest UV (0.5, 0), samples around y=0.05 in source UV.
    int corners[8] = {
        96, 0,
        1823, 0,
        1919, 1079,
        0, 1079,
    };
    float h[9] = {};
    auto s = homography::corners_to_warp(corners, 1920, 1080, h);
    ASSERT_EQ(homography::Status::Ok, s);

    auto top_mid = apply(h, 0.5f, 0.0f);
    // The top edge of the dest unit-square maps to a horizontal line
    // 5% inset from the source's top edge.
    EXPECT_NEAR(top_mid[0], 0.5f, 1e-3f);
    EXPECT_NEAR(top_mid[1], 0.0f, 1e-3f);

    auto bot_mid = apply(h, 0.5f, 1.0f);
    EXPECT_NEAR(bot_mid[0], 0.5f, 1e-3f);
    EXPECT_NEAR(bot_mid[1], 1079.0f / 1080.0f, 1e-3f);
}

TEST(Homography, RoundTripCornersThroughMatrix) {
    // For any non-degenerate set of corners, applying the matrix to the
    // dest unit-square's four vertices should reproduce the source
    // corners (up to numeric tolerance). Strongest guarantee that the
    // solve is correct.
    int corners[8] = {
        200, 100,
        1700, 80,
        1750, 950,
        180, 1000,
    };
    float h[9] = {};
    auto s = homography::corners_to_warp(corners, 1920, 1080, h);
    ASSERT_EQ(homography::Status::Ok, s);

    auto tl = apply(h, 0.0f, 0.0f);
    auto tr = apply(h, 1.0f, 0.0f);
    auto br = apply(h, 1.0f, 1.0f);
    auto bl = apply(h, 0.0f, 1.0f);
    EXPECT_NEAR(tl[0], 200.0f / 1920.0f, 1e-4f);
    EXPECT_NEAR(tl[1], 100.0f / 1080.0f, 1e-4f);
    EXPECT_NEAR(tr[0], 1700.0f / 1920.0f, 1e-4f);
    EXPECT_NEAR(tr[1], 80.0f / 1080.0f, 1e-4f);
    EXPECT_NEAR(br[0], 1750.0f / 1920.0f, 1e-4f);
    EXPECT_NEAR(br[1], 950.0f / 1080.0f, 1e-4f);
    EXPECT_NEAR(bl[0], 180.0f / 1920.0f, 1e-4f);
    EXPECT_NEAR(bl[1], 1000.0f / 1080.0f, 1e-4f);
}

TEST(Homography, RejectsZeroSnapshotDims) {
    int corners[8] = {0, 0, 1919, 0, 1919, 1079, 0, 1079};
    float h[9] = {};
    EXPECT_EQ(homography::Status::BadSnapshotDims,
              homography::corners_to_warp(corners, 0, 1080, h));
    EXPECT_EQ(homography::Status::BadSnapshotDims,
              homography::corners_to_warp(corners, 1920, 0, h));
    EXPECT_EQ(homography::Status::BadSnapshotDims,
              homography::corners_to_warp(corners, -1, 1080, h));
}

TEST(Homography, RejectsCollinearCorners) {
    // Three points on the top edge + one elsewhere → no unique homography
    // (the linear system is rank-deficient).
    int corners[8] = {
        0, 0,
        960, 0,
        1919, 0,
        0, 1079,
    };
    float h[9] = {};
    EXPECT_EQ(homography::Status::Degenerate,
              homography::corners_to_warp(corners, 1920, 1080, h));
}

TEST(Homography, RejectsCoincidentCorners) {
    // Two corners at the same point also degenerate.
    int corners[8] = {
        0, 0,
        0, 0,
        1919, 1079,
        0, 1079,
    };
    float h[9] = {};
    EXPECT_EQ(homography::Status::Degenerate,
              homography::corners_to_warp(corners, 1920, 1080, h));
}
