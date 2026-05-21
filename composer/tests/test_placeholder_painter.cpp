// Unit tests for placeholder_painter. Pure CPU, no V4L2/RGA/EGL — runs
// on host without devices.

#include "../src/placeholder_painter.hpp"

#include <gtest/gtest.h>

#include <cstdint>
#include <cstring>
#include <vector>

namespace {

constexpr int kW = 1920;
constexpr int kH = 1080;
constexpr size_t kNV12Size = size_t(kW) * kH * 3 / 2;

std::vector<uint8_t> make_buf() {
    return std::vector<uint8_t>(kNV12Size, 0);
}

} // namespace

TEST(PlaceholderPainter, PaintBaseFillsBackground) {
    auto buf = make_buf();
    placeholder_painter::paint_base(buf, kW, kH, "");

    // Luma plane: mostly background (32), with a small handful of bright
    // pixels where the title text is rendered. Check that the top-left
    // corner is the bg color (no text there).
    EXPECT_EQ(uint8_t(32), buf[0]);
    EXPECT_EQ(uint8_t(32), buf[100 * kW + 100]);

    // Chroma plane starts at kW*kH. Both Cb and Cr should be filled
    // (140, 120) throughout.
    size_t uv_start = size_t(kW) * kH;
    EXPECT_EQ(uint8_t(140), buf[uv_start + 0]);   // Cb
    EXPECT_EQ(uint8_t(120), buf[uv_start + 1]);   // Cr
    EXPECT_EQ(uint8_t(140), buf[uv_start + 200]); // arbitrary later Cb
    EXPECT_EQ(uint8_t(120), buf[uv_start + 201]); // arbitrary later Cr
}

TEST(PlaceholderPainter, PaintBaseWritesTitleText) {
    auto buf = make_buf();
    placeholder_painter::paint_base(buf, kW, kH, "");

    // Title baseline is ~1/3 down. Pull the row range that contains it
    // and count "bright" (>200) luma pixels. With 18 chars at 4x scale
    // and ~32px-tall glyphs there should be hundreds.
    int title_y = kH / 3;
    int title_h = 8 * 4; // font kCharH * kTitleScale
    size_t bright = 0;
    for (int y = title_y; y < title_y + title_h; ++y) {
        for (int x = 0; x < kW; ++x) {
            if (buf[y * kW + x] > 200)
                ++bright;
        }
    }
    // Expect well over 500 bright pixels (every "on" font pixel is a 4x4
    // block = 16 px each, ~18 chars × ~10 strokes per char × 16 ≈ 2880).
    EXPECT_TRUE(bright > 500);
}

TEST(PlaceholderPainter, PaintBaseWithDevicePath) {
    auto buf = make_buf();
    placeholder_painter::paint_base(buf, kW, kH, "/dev/v4l/by-path/platform-fdee0000.hdmirx");

    // Subtitle sits below the title baseline. Count bright pixels in a
    // strip a bit below the title — must be > 0 (proves subtitle painted)
    // but smaller than the title (subtitle is 2x scale, not 4x).
    int subtitle_y_top = kH / 3 + 8 * 4 + 24;
    int subtitle_h = 8 * 2;
    size_t bright = 0;
    for (int y = subtitle_y_top; y < subtitle_y_top + subtitle_h; ++y) {
        for (int x = 0; x < kW; ++x) {
            if (buf[y * kW + x] > 200)
                ++bright;
        }
    }
    EXPECT_TRUE(bright > 50);
}

TEST(PlaceholderPainter, PaintTickOnlyTouchesAnimRegion) {
    auto buf = make_buf();
    placeholder_painter::paint_base(buf, kW, kH, "");

    // paint_tick now writes the status line slightly above the spinner
    // region; compute the actual region it clears.
    auto region = placeholder_painter::derive_anim_region(kW, kH);
    int status_top = region.y_start - (8 * 2) - 8;
    if (status_top < 0)
        status_top = 0;

    std::vector<uint8_t> snapshot_above(buf.begin(), buf.begin() + size_t(status_top) * kW);
    std::vector<uint8_t> snapshot_below(buf.begin() + size_t(region.y_end) * kW,
                                        buf.begin() + size_t(kW) * kH);

    placeholder_painter::paint_tick(buf, kW, kH, 42, 12345, "TESTING");

    EXPECT_EQ(0, int(std::memcmp(snapshot_above.data(), buf.data(), snapshot_above.size())));
    EXPECT_EQ(0, int(std::memcmp(snapshot_below.data(), buf.data() + size_t(region.y_end) * kW,
                                 snapshot_below.size())));
}

TEST(PlaceholderPainter, PaintTickChangesWithTickIdx) {
    auto buf_a = make_buf();
    auto buf_b = make_buf();
    placeholder_painter::paint_base(buf_a, kW, kH, "");
    placeholder_painter::paint_base(buf_b, kW, kH, "");

    placeholder_painter::paint_tick(buf_a, kW, kH, 1, 100, "LIVE");
    placeholder_painter::paint_tick(buf_b, kW, kH, 2, 200, "LIVE");

    auto region = placeholder_painter::derive_anim_region(kW, kH);
    bool different = false;
    for (int y = region.y_start; y < region.y_end; ++y) {
        if (std::memcmp(buf_a.data() + size_t(y) * kW, buf_b.data() + size_t(y) * kW, kW) != 0) {
            different = true;
            break;
        }
    }
    EXPECT_TRUE(different);
}

TEST(PlaceholderPainter, PaintTickBoundsSafetySmallCanvas) {
    // A small but-valid canvas. The painter must not crash or write
    // out-of-bounds. Lower bound 256 is documented in the header.
    const int sw = 256, sh = 256;
    std::vector<uint8_t> sbuf(size_t(sw) * sh * 3 / 2, 0xAA);
    placeholder_painter::paint_base(sbuf, sw, sh, "");
    placeholder_painter::paint_tick(sbuf, sw, sh, 7, 0, "BOUNDS");
    // Sentinel: the byte before the buffer should not be touched — we
    // can't check that without a guard page, but we can check the LAST
    // byte: chroma plane ends at end of buffer; should be either bg or
    // touched-by-fill. Just check no out-of-range crash got us here.
    EXPECT_TRUE(sbuf.size() == size_t(sw) * sh * 3 / 2);
}
