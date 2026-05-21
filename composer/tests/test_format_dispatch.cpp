// Tests for format_dispatch. Pure logic, no EGL/GBM/dma-buf — runs on host.

#include "../src/format_dispatch.hpp"
#include "../src/egl_ctx.hpp"

#include <gtest/gtest.h>

#include <drm_fourcc.h>

using format_dispatch::fill_image_desc;
using format_dispatch::fourcc_from_string;

TEST(FormatDispatch, FourccFromString) {
    EXPECT_EQ(uint32_t(DRM_FORMAT_NV12), fourcc_from_string("NV12"));
    EXPECT_EQ(uint32_t(DRM_FORMAT_NV16), fourcc_from_string("NV16"));
    EXPECT_EQ(uint32_t(DRM_FORMAT_NV24), fourcc_from_string("NV24"));
    EXPECT_EQ(uint32_t(DRM_FORMAT_BGR888), fourcc_from_string("BG24"));
    EXPECT_EQ(uint32_t(0), fourcc_from_string(""));
    EXPECT_EQ(uint32_t(0), fourcc_from_string("XYZ"));     // wrong length
    EXPECT_EQ(uint32_t(0), fourcc_from_string("TOOLONG")); // wrong length
}

TEST(FormatDispatch, FillNv12DerivesChroma) {
    egl_ctx::EglCtx::ImageDesc d{};
    fill_image_desc(d, "NV12", 320, 240);
    EXPECT_EQ(uint32_t(DRM_FORMAT_NV12), d.fourcc);
    EXPECT_EQ(320, d.plane0_pitch);
    EXPECT_EQ(0, d.plane0_offset);
    EXPECT_EQ(320, d.plane1_pitch);        // CbCr row = W (interleaved pairs)
    EXPECT_EQ(320 * 240, d.plane1_offset); // after luma
}

TEST(FormatDispatch, FillNv16DerivesChroma) {
    egl_ctx::EglCtx::ImageDesc d{};
    fill_image_desc(d, "NV16", 320, 240);
    EXPECT_EQ(uint32_t(DRM_FORMAT_NV16), d.fourcc);
    EXPECT_EQ(320, d.plane0_pitch);
    EXPECT_EQ(320, d.plane1_pitch);
    EXPECT_EQ(320 * 240, d.plane1_offset);
}

TEST(FormatDispatch, FillNv24DerivesChromaFullWidth) {
    egl_ctx::EglCtx::ImageDesc d{};
    fill_image_desc(d, "NV24", 320, 240);
    EXPECT_EQ(uint32_t(DRM_FORMAT_NV24), d.fourcc);
    EXPECT_EQ(320, d.plane0_pitch);
    EXPECT_EQ(320 * 2, d.plane1_pitch); // 2 bytes/pixel CbCr at full W
    EXPECT_EQ(320 * 240, d.plane1_offset);
}

TEST(FormatDispatch, FillBg24LeavesPlane1Zero) {
    egl_ctx::EglCtx::ImageDesc d{};
    d.plane0_pitch = 320 * 3; // caller is expected to set this for packed RGB
    fill_image_desc(d, "BG24", 320, 240);
    EXPECT_EQ(uint32_t(DRM_FORMAT_BGR888), d.fourcc);
    EXPECT_EQ(320 * 3, d.plane0_pitch);
    EXPECT_EQ(0, d.plane1_pitch);
    EXPECT_EQ(0, d.plane1_offset);
}

TEST(FormatDispatch, EmptyFormatDefaultsToNv12) {
    egl_ctx::EglCtx::ImageDesc d{};
    fill_image_desc(d, "", 64, 32);
    EXPECT_EQ(uint32_t(DRM_FORMAT_NV12), d.fourcc);
    EXPECT_EQ(64, d.plane1_pitch);
    EXPECT_EQ(64 * 32, d.plane1_offset);
}

TEST(FormatDispatch, CallerProvidedPlane1IsRespected) {
    egl_ctx::EglCtx::ImageDesc d{};
    d.plane1_pitch = 999;
    d.plane1_offset = 7777;
    fill_image_desc(d, "NV12", 320, 240);
    EXPECT_EQ(999, d.plane1_pitch); // not overwritten
    EXPECT_EQ(7777, d.plane1_offset);
}
