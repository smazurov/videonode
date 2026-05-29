// Tests for build_source_slot — the pure frame+layout → SourceSlot
// mapping. No EGL/GBM/dma-buf; runs on host.

#include "src/render/build_source_slot.hpp"

#include <gtest/gtest.h>

using render::build_source_slot;
using render::FrameGeom;
using render::LayoutRect;
using render::SourceState;

namespace {

// A 1080p source placed 1:1 into a same-size slot, no aspect-ratio mode.
FrameGeom hd_frame() {
    FrameGeom f;
    f.y_fd = 7;
    f.uv_fd = 9;
    f.width = 1920;
    f.height = 1080;
    f.y_pitch = 1920;
    f.uv_pitch = 1920;
    f.y_offset = 0;
    f.uv_offset = 0;
    return f;
}

LayoutRect full_rect() {
    LayoutRect r;
    r.x = 0;
    r.y = 0;
    r.w = 1920;
    r.h = 1080;
    return r;
}

} // namespace

TEST(BuildSourceSlot, FdAndDimsPassthrough) {
    auto s = build_source_slot(hd_frame(), full_rect(), nullptr);
    EXPECT_EQ(7, s.src_y_fd);
    EXPECT_EQ(9, s.src_uv_fd);
    EXPECT_EQ(1920, s.src_w);
    EXPECT_EQ(1080, s.src_h);
    EXPECT_EQ(0, s.x);
    EXPECT_EQ(0, s.y);
    EXPECT_EQ(1920, s.w);
    EXPECT_EQ(1080, s.h);
}

TEST(BuildSourceSlot, PitchPassthroughAndDerive) {
    FrameGeom f = hd_frame();
    f.y_pitch = 2048; // hardware-aligned stride > width
    f.uv_pitch = 2048;
    auto s = build_source_slot(f, full_rect(), nullptr);
    EXPECT_EQ(2048, s.src_y_pitch);
    EXPECT_EQ(2048, s.src_uv_pitch);

    f.y_pitch = 0; // 0 → derive from width
    f.uv_pitch = 0;
    auto s2 = build_source_slot(f, full_rect(), nullptr);
    EXPECT_EQ(1920, s2.src_y_pitch);
    EXPECT_EQ(1920, s2.src_uv_pitch);
}

// Regression: single-buffer NV12 from videonode-source carries Y and UV
// in one dma-buf with the UV plane at a hardware-aligned byte offset
// (height padded to a multiple of 16: 1080 → 1088). The slot must carry
// that offset through to the compositor; dropping it makes pl_compose
// sample chroma from the top of the luma plane → magenta/green ghost.
TEST(BuildSourceSlot, PlaneByteOffsetsPassthrough) {
    FrameGeom f = hd_frame();
    f.y_offset = 0;
    f.uv_offset = 1920 * 1088; // UV after 1088 padded luma rows
    auto s = build_source_slot(f, full_rect(), nullptr);
    EXPECT_EQ(0, s.src_y_offset);
    EXPECT_EQ(1920 * 1088, s.src_uv_offset);
}

TEST(BuildSourceSlot, FitLetterboxesWideSourceIntoSquareSlot) {
    LayoutRect r;
    r.x = 0;
    r.y = 0;
    r.w = 1000;
    r.h = 1000;
    r.aspect_ratio_mode = 1; // fit
    auto s = build_source_slot(hd_frame(), r, nullptr);
    // 16:9 into 1:1 → full width, shrunk height centred vertically.
    EXPECT_EQ(0, s.x);
    EXPECT_EQ(1000, s.w);
    EXPECT_EQ(562, s.h); // 1000 / (1920/1080)
    EXPECT_EQ(219, s.y); // (1000 - 562) / 2
}

TEST(BuildSourceSlot, CropFillsSquareSlotFromWideSourceCentred) {
    LayoutRect r;
    r.x = 0;
    r.y = 0;
    r.w = 1000;
    r.h = 1000;
    r.aspect_ratio_mode = 2; // crop
    r.crop_x = 0.5f;
    r.crop_y = 0.5f;
    r.crop_scale = 1.0f;
    auto s = build_source_slot(hd_frame(), r, nullptr);
    // Visible width = (1/1.7778) = 0.5625 of source, centred.
    EXPECT_NEAR(0.21875f, s.src_crop_x0, 1e-4f);
    EXPECT_NEAR(0.78125f, s.src_crop_x1, 1e-4f);
    EXPECT_NEAR(0.0f, s.src_crop_y0, 1e-4f);
    EXPECT_NEAR(1.0f, s.src_crop_y1, 1e-4f);
}
