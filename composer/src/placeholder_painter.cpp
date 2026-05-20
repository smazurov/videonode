#include "placeholder_painter.hpp"

#include "font8x8.h"

#include <algorithm>
#include <cstdio>
#include <cstring>
#include <string>

namespace placeholder_painter {

namespace {

// NV12 background palette. Dark navy chosen to be obviously "system on"
// without being loud. Luma 32 (dark), Cb 140 / Cr 120 → faint cool tint.
constexpr uint8_t kBgY = 32;
constexpr uint8_t kBgCb = 140;
constexpr uint8_t kBgCr = 120;
constexpr uint8_t kFgY = 235; // "white" in limited range — matches what
                              // panfrost-sampled BT.601 expects later

// Title text height / scale knobs. Scale is integer pixel-replication of
// the 8x8 font.
constexpr int kTitleScale = 4;     // 32px-tall title text
constexpr int kTickScale = 2;      // 16px-tall timestamp text
constexpr int kSpinnerRadius = 24; // pixels
constexpr int kSpinnerDots = 8;

const char kTitle[] = "NO SIGNAL DETECTED";

// fill_luma fills a rectangular region of the luma plane with a constant
// value. Region is clipped to [0,w) x [0,h).
void fill_luma(uint8_t* y_plane, int w, int h, int x0, int y0, int x1, int y1, uint8_t v) {
    x0 = std::max(0, x0);
    y0 = std::max(0, y0);
    x1 = std::min(w, x1);
    y1 = std::min(h, y1);
    if (x0 >= x1 || y0 >= y1)
        return;
    for (int y = y0; y < y1; ++y) {
        std::memset(y_plane + y * w + x0, v, size_t(x1 - x0));
    }
}

// fill_chroma fills the interleaved CbCr plane (half W, half H for NV12)
// at the chroma pixel position derived from the luma rectangle.
void fill_chroma(uint8_t* uv_plane, int w, int h_y, int x0, int y0, int x1, int y1, uint8_t cb,
                 uint8_t cr) {
    // NV12 chroma: one CbCr pair per 2x2 luma block. uv stride = w bytes
    // (w/2 pairs * 2 bytes each), uv height = h_y/2.
    int cx0 = std::max(0, x0 / 2);
    int cx1 = std::min(w / 2, (x1 + 1) / 2);
    int cy0 = std::max(0, y0 / 2);
    int cy1 = std::min(h_y / 2, (y1 + 1) / 2);
    for (int cy = cy0; cy < cy1; ++cy) {
        uint8_t* row = uv_plane + cy * w + cx0 * 2;
        for (int cx = cx0; cx < cx1; ++cx) {
            *row++ = cb;
            *row++ = cr;
        }
    }
}

// draw_glyph_luma stamps one font8x8 glyph at (px, py), scaled by `scale`
// (integer pixel replication). Bits with MSB=left convention.
void draw_glyph_luma(uint8_t* y_plane, int w, int h, int px, int py, int scale, uint8_t value,
                     char ch) {
    if (ch < 0 || ch > 0x7F)
        ch = ' ';
    for (int row = 0; row < font8x8::kCharH; ++row) {
        uint8_t bits = font8x8::kData[(int)ch][row];
        for (int col = 0; col < font8x8::kCharW; ++col) {
            if (bits & (0x80 >> col)) {
                int x0 = px + col * scale;
                int y0 = py + row * scale;
                fill_luma(y_plane, w, h, x0, y0, x0 + scale, y0 + scale, value);
            }
        }
    }
}

// draw_text_luma writes a string with the given scale, centered around px.
// Returns the right edge (for chaining if needed).
int draw_text_luma(uint8_t* y_plane, int w, int h, int px, int py, int scale, uint8_t value,
                   const char* s, int n) {
    int char_w = font8x8::kCharW * scale;
    int total = char_w * n;
    int x = px - total / 2;
    for (int i = 0; i < n; ++i) {
        draw_glyph_luma(y_plane, w, h, x + i * char_w, py, scale, value, s[i]);
    }
    return x + total;
}

} // namespace

AnimRegion derive_anim_region(int w, int h) {
    (void)w;
    // Title sits ~1/3 down. Subtitle (device path) adds ~16-24 px below.
    // Animation strip goes below that, leaving room for timestamp + spinner.
    int title_baseline = h / 3 + font8x8::kCharH * kTitleScale;
    int subtitle_h = font8x8::kCharH * kTickScale + 24;
    int anim_y_start = title_baseline + subtitle_h + 24;
    int anim_y_end = anim_y_start + (kSpinnerRadius * 2) + 80;
    return AnimRegion{anim_y_start, anim_y_end};
}

void paint_base(uint8_t* nv12, int w, int h, const char* device_path) {
    uint8_t* y_plane = nv12;
    uint8_t* uv_plane = nv12 + (w * h);

    std::memset(y_plane, kBgY, size_t(w) * h);
    {
        const int half_h = h / 2;
        for (int cy = 0; cy < half_h; ++cy) {
            uint8_t* row = uv_plane + cy * w;
            for (int cx = 0; cx < w / 2; ++cx) {
                *row++ = kBgCb;
                *row++ = kBgCr;
            }
        }
    }

    int title_chars = int(sizeof(kTitle)) - 1;
    int title_x_center = w / 2;
    int title_y = h / 3;
    draw_text_luma(y_plane, w, h, title_x_center, title_y, kTitleScale, kFgY, kTitle, title_chars);

    // Device-path subtitle: font is uppercase-only, so upper-case the
    // string before drawing. Truncate to fit canvas width at scale 2.
    if (device_path && *device_path) {
        int sub_scale = 2;
        int char_w = font8x8::kCharW * sub_scale;
        int max_chars = (w - 32) / char_w;
        char sub[256];
        int n = 0;
        for (; device_path[n] && n < int(sizeof(sub)) - 1 && n < max_chars; ++n) {
            char c = device_path[n];
            if (c >= 'a' && c <= 'z')
                c = char(c - 'a' + 'A');
            sub[n] = c;
        }
        sub[n] = 0;
        int sub_y = title_y + font8x8::kCharH * kTitleScale + 24;
        draw_text_luma(y_plane, w, h, w / 2, sub_y, sub_scale, kFgY, sub, n);
    }
}

void paint_tick(uint8_t* nv12, int w, int h, uint64_t tick_idx, uint64_t wallclock_ms,
                const char* status) {
    uint8_t* y_plane = nv12;
    AnimRegion r = derive_anim_region(w, h);

    // Clear a strip extending a bit above r.y_start to include the
    // status-line row (paint_tick is allowed to write there too).
    int status_top = r.y_start - (font8x8::kCharH * kTickScale) - 8;
    if (status_top < 0)
        status_top = 0;
    fill_luma(y_plane, w, h, 0, status_top, w, r.y_end, kBgY);

    if (status && *status) {
        int n = 0;
        while (status[n] && n < 64)
            ++n;
        draw_text_luma(y_plane, w, h, w / 2, status_top + 4, kTickScale, kFgY, status, n);
    }

    // Timestamp: "FRAME NNNNNNN  HH MM SS  MMM" — no punctuation since
    // font8x8 only has 0-9, A-Z, '.', ':', '*', ' '.
    uint64_t ms_total = wallclock_ms;
    int ms = int(ms_total % 1000);
    uint64_t s_total = ms_total / 1000;
    int s = int(s_total % 60);
    int m = int((s_total / 60) % 60);
    int hh = int((s_total / 3600) % 24);

    char buf[64];
    int n = std::snprintf(buf, sizeof(buf), "FRAME %07llu   %02d:%02d:%02d.%03d",
                          static_cast<unsigned long long>(tick_idx), hh, m, s, ms);

    // Convert to uppercase-friendly (snprintf already uses digits + colons + dot).
    int text_y = r.y_start + 4;
    draw_text_luma(y_plane, w, h, w / 2, text_y, kTickScale, kFgY, buf, n);

    // Spinner: 8 dots in a circle, one bright per tick (rotates clockwise).
    // Sits a comfortable gap below the text — too close looks crowded.
    int spinner_cy = r.y_start + (font8x8::kCharH * kTickScale) + 48 + kSpinnerRadius;
    int spinner_cx = w / 2;
    // Spinner advances one dot per kSpinnerTickDivisor frames. With the
    // default placeholder fps of 30, divisor=4 → ~1 revolution/sec, the
    // common "I'm thinking" pace. Faster looked frantic for a "system
    // waiting" indicator.
    constexpr int kSpinnerTickDivisor = 4;
    int active_dot = int((tick_idx / kSpinnerTickDivisor) % kSpinnerDots);
    for (int i = 0; i < kSpinnerDots; ++i) {
        // Convert dot index to angle. 0 at top, going clockwise.
        // angle_rad = -pi/2 + i * (2pi/8) = i * pi/4 - pi/2
        // x = cx + r*cos(angle), y = cy + r*sin(angle)
        // Precomputed cos/sin for 8 dots:
        static const int kDx[8] = {0, 17, 24, 17, 0, -17, -24, -17};
        static const int kDy[8] = {-24, -17, 0, 17, 24, 17, 0, -17};
        int dot_x = spinner_cx + kDx[i];
        int dot_y = spinner_cy + kDy[i];
        uint8_t v = (i == active_dot) ? kFgY : (kBgY + 24);
        fill_luma(y_plane, w, h, dot_x - 3, dot_y - 3, dot_x + 3, dot_y + 3, v);
    }
}

} // namespace placeholder_painter
