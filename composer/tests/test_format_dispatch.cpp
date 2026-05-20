// Tests for format_dispatch. Pure logic, no EGL/GBM/dma-buf — runs on host.

#include "../src/format_dispatch.hpp"
#include "../src/egl_ctx.hpp"
#include "test_runner.hpp"

#include <drm_fourcc.h>

using format_dispatch::fill_image_desc;
using format_dispatch::fourcc_from_string;

static void test_fourcc_from_string() {
    CHECK_EQ(uint32_t(DRM_FORMAT_NV12), fourcc_from_string("NV12"));
    CHECK_EQ(uint32_t(DRM_FORMAT_NV16), fourcc_from_string("NV16"));
    CHECK_EQ(uint32_t(DRM_FORMAT_NV24), fourcc_from_string("NV24"));
    CHECK_EQ(uint32_t(DRM_FORMAT_BGR888), fourcc_from_string("BG24"));
    CHECK_EQ(uint32_t(0), fourcc_from_string(""));
    CHECK_EQ(uint32_t(0), fourcc_from_string("XYZ"));     // wrong length
    CHECK_EQ(uint32_t(0), fourcc_from_string("TOOLONG")); // wrong length
}

static void test_fill_nv12_derives_chroma() {
    egl_ctx::EglCtx::ImageDesc d{};
    fill_image_desc(d, "NV12", 320, 240);
    CHECK_EQ(uint32_t(DRM_FORMAT_NV12), d.fourcc);
    CHECK_EQ(320, d.plane0_pitch);
    CHECK_EQ(0, d.plane0_offset);
    CHECK_EQ(320, d.plane1_pitch);        // CbCr row = W (interleaved pairs)
    CHECK_EQ(320 * 240, d.plane1_offset); // after luma
}

static void test_fill_nv16_derives_chroma() {
    egl_ctx::EglCtx::ImageDesc d{};
    fill_image_desc(d, "NV16", 320, 240);
    CHECK_EQ(uint32_t(DRM_FORMAT_NV16), d.fourcc);
    CHECK_EQ(320, d.plane0_pitch);
    CHECK_EQ(320, d.plane1_pitch);
    CHECK_EQ(320 * 240, d.plane1_offset);
}

static void test_fill_nv24_derives_chroma_full_width() {
    egl_ctx::EglCtx::ImageDesc d{};
    fill_image_desc(d, "NV24", 320, 240);
    CHECK_EQ(uint32_t(DRM_FORMAT_NV24), d.fourcc);
    CHECK_EQ(320, d.plane0_pitch);
    CHECK_EQ(320 * 2, d.plane1_pitch); // 2 bytes/pixel CbCr at full W
    CHECK_EQ(320 * 240, d.plane1_offset);
}

static void test_fill_bg24_leaves_plane1_zero() {
    egl_ctx::EglCtx::ImageDesc d{};
    d.plane0_pitch = 320 * 3; // caller is expected to set this for packed RGB
    fill_image_desc(d, "BG24", 320, 240);
    CHECK_EQ(uint32_t(DRM_FORMAT_BGR888), d.fourcc);
    CHECK_EQ(320 * 3, d.plane0_pitch);
    CHECK_EQ(0, d.plane1_pitch);
    CHECK_EQ(0, d.plane1_offset);
}

static void test_empty_format_defaults_to_nv12() {
    egl_ctx::EglCtx::ImageDesc d{};
    fill_image_desc(d, "", 64, 32);
    CHECK_EQ(uint32_t(DRM_FORMAT_NV12), d.fourcc);
    CHECK_EQ(64, d.plane1_pitch);
    CHECK_EQ(64 * 32, d.plane1_offset);
}

static void test_caller_provided_plane1_is_respected() {
    egl_ctx::EglCtx::ImageDesc d{};
    d.plane1_pitch = 999;
    d.plane1_offset = 7777;
    fill_image_desc(d, "NV12", 320, 240);
    CHECK_EQ(999, d.plane1_pitch); // not overwritten
    CHECK_EQ(7777, d.plane1_offset);
}

int main() {
    test_runner::start_case("fourcc_from_string");
    test_fourcc_from_string();
    test_runner::start_case("fill_nv12_derives_chroma");
    test_fill_nv12_derives_chroma();
    test_runner::start_case("fill_nv16_derives_chroma");
    test_fill_nv16_derives_chroma();
    test_runner::start_case("fill_nv24_derives_chroma_full_width");
    test_fill_nv24_derives_chroma_full_width();
    test_runner::start_case("fill_bg24_leaves_plane1_zero");
    test_fill_bg24_leaves_plane1_zero();
    test_runner::start_case("empty_format_defaults_to_nv12");
    test_empty_format_defaults_to_nv12();
    test_runner::start_case("caller_provided_plane1_is_respected");
    test_caller_provided_plane1_is_respected();
    return test_runner::report_and_exit_code();
}
