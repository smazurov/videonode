#include "src/capture/v4l2_capture.hpp"

#include <gtest/gtest.h>

#include <linux/videodev2.h>

using v4l2::ColorMatrix;
using v4l2::resolve_matrix;

TEST(ResolveMatrix, ExplicitYcbcrEnc709Wins) {
    EXPECT_EQ(resolve_matrix(V4L2_COLORSPACE_SMPTE170M, V4L2_YCBCR_ENC_709, 480),
              ColorMatrix::Bt709);
}

TEST(ResolveMatrix, ExplicitYcbcrEnc601WinsAt1080p) {
    EXPECT_EQ(resolve_matrix(V4L2_COLORSPACE_SRGB, V4L2_YCBCR_ENC_601, 1080), ColorMatrix::Bt601);
}

TEST(ResolveMatrix, Rec709ColorspaceMapsTo709) {
    EXPECT_EQ(resolve_matrix(V4L2_COLORSPACE_REC709, V4L2_YCBCR_ENC_DEFAULT, 480),
              ColorMatrix::Bt709);
}

TEST(ResolveMatrix, Smpte170mColorspaceMapsTo601) {
    EXPECT_EQ(resolve_matrix(V4L2_COLORSPACE_SMPTE170M, V4L2_YCBCR_ENC_DEFAULT, 1080),
              ColorMatrix::Bt601);
}

TEST(ResolveMatrix, Bt470SystemsMapTo601) {
    EXPECT_EQ(resolve_matrix(V4L2_COLORSPACE_470_SYSTEM_M, V4L2_YCBCR_ENC_DEFAULT, 1080),
              ColorMatrix::Bt601);
    EXPECT_EQ(resolve_matrix(V4L2_COLORSPACE_470_SYSTEM_BG, V4L2_YCBCR_ENC_DEFAULT, 1080),
              ColorMatrix::Bt601);
}

TEST(ResolveMatrix, DefaultColorspaceHdHeuristicIs709) {
    EXPECT_EQ(resolve_matrix(V4L2_COLORSPACE_DEFAULT, V4L2_YCBCR_ENC_DEFAULT, 720),
              ColorMatrix::Bt709);
    EXPECT_EQ(resolve_matrix(V4L2_COLORSPACE_SRGB, V4L2_YCBCR_ENC_DEFAULT, 1080),
              ColorMatrix::Bt709);
}

TEST(ResolveMatrix, DefaultColorspaceSdHeuristicIs601) {
    EXPECT_EQ(resolve_matrix(V4L2_COLORSPACE_DEFAULT, V4L2_YCBCR_ENC_DEFAULT, 480),
              ColorMatrix::Bt601);
    EXPECT_EQ(resolve_matrix(V4L2_COLORSPACE_SRGB, V4L2_YCBCR_ENC_DEFAULT, 576),
              ColorMatrix::Bt601);
}

TEST(ResolveMatrix, HeuristicBoundaryAt720) {
    EXPECT_EQ(resolve_matrix(V4L2_COLORSPACE_DEFAULT, V4L2_YCBCR_ENC_DEFAULT, 719),
              ColorMatrix::Bt601);
    EXPECT_EQ(resolve_matrix(V4L2_COLORSPACE_DEFAULT, V4L2_YCBCR_ENC_DEFAULT, 720),
              ColorMatrix::Bt709);
}
