#include "src/render/placeholder_painter.hpp"

#include "font8x8.h"

#include <algorithm>
#include <cstdio>
#include <cstring>
#include <span>
#include <string>
#include <string_view>

namespace placeholder_painter {

namespace {

// NV12 background palette. Dark navy chosen to be obviously "system on"
// without being loud. Luma 32 (dark), Cb 140 / Cr 120 → faint cool tint.
constexpr uint8_t kBgY = 32;
constexpr uint8_t kBgCb = 140;
constexpr uint8_t kBgCr = 120;
constexpr uint8_t kFgY = 235; // "white" in limited range — matches what
                              // panfrost-sampled BT.601 expects later

constexpr int kTitleScale = 4;
constexpr int kTickScale = 2;
constexpr int kSpinnerRadius = 24;
constexpr int kSpinnerDots = 8;

const char kTitle[] = "NO SIGNAL DETECTED";

struct PlaneInfo {
    std::span<uint8_t> plane;
    int w;
    int h;
};

struct DrawParams {
    int px;
    int py;
    int scale;
    uint8_t value;
};

void fill_luma(PlaneInfo pi, int x0, int y0, int x1, int y1, uint8_t v) {
    x0 = std::max(0, x0);
    y0 = std::max(0, y0);
    x1 = std::min(pi.w, x1);
    y1 = std::min(pi.h, y1);
    if (x0 >= x1 || y0 >= y1)
        return;
    for (int y = y0; y < y1; ++y) {
        auto row =
            pi.plane.subspan(static_cast<size_t>(y * pi.w + x0), static_cast<size_t>(x1 - x0));
        std::memset(row.data(), v, row.size());
    }
}

// `ch` is `int` (not `char`) so the 0/0x7F bounds check is well-defined
// regardless of whether plain `char` is signed or unsigned on the target.
void draw_glyph_luma(PlaneInfo pi, DrawParams dp, int ch) {
    if (ch < 0 || ch > 0x7F)
        ch = ' ';
    for (int row = 0; row < font8x8::kCharH; ++row) {
        uint8_t bits = font8x8::kData[ch][row];
        for (int col = 0; col < font8x8::kCharW; ++col) {
            if (bits & (0x80 >> col)) {
                int x0 = dp.px + col * dp.scale;
                int y0 = dp.py + row * dp.scale;
                fill_luma(pi, x0, y0, x0 + dp.scale, y0 + dp.scale, dp.value);
            }
        }
    }
}

int draw_text_luma(PlaneInfo pi, DrawParams dp, std::string_view s) {
    int char_w = font8x8::kCharW * dp.scale;
    int total = char_w * static_cast<int>(s.size());
    int x = dp.px - total / 2;
    for (int i = 0; i < static_cast<int>(s.size()); ++i) {
        draw_glyph_luma(
            pi, DrawParams{.px = x + i * char_w, .py = dp.py, .scale = dp.scale, .value = dp.value},
            static_cast<unsigned char>(s[i]));
    }
    return x + total;
}

} // namespace

AnimRegion derive_anim_region(int w, int h) {
    (void)w;
    int title_baseline = h / 3 + font8x8::kCharH * kTitleScale;
    int subtitle_h = font8x8::kCharH * kTickScale + 24;
    int anim_y_start = title_baseline + subtitle_h + 24;
    int anim_y_end = anim_y_start + (kSpinnerRadius * 2) + 80;
    return AnimRegion{.y_start = anim_y_start, .y_end = anim_y_end};
}

bool paint_base(std::span<uint8_t> nv12, int w, int h, const char* device_path) {
    const std::size_t need = static_cast<std::size_t>(w) * static_cast<std::size_t>(h) * 3 / 2;
    if (w <= 0 || h <= 0 || nv12.size() < need)
        return false;

    const size_t y_size = static_cast<size_t>(w) * h;
    std::span<uint8_t> y_span = nv12.subspan(0, y_size);
    std::span<uint8_t> uv_span = nv12.subspan(y_size);

    PlaneInfo pi{.plane = y_span, .w = w, .h = h};

    std::memset(y_span.data(), kBgY, y_span.size());
    {
        const int half_h = h / 2;
        for (int cy = 0; cy < half_h; ++cy) {
            auto row = uv_span.subspan(static_cast<size_t>(cy * w));
            size_t idx = 0;
            for (int cx = 0; cx < w / 2; ++cx) {
                row[idx++] = kBgCb;
                row[idx++] = kBgCr;
            }
        }
    }

    int title_chars = int(sizeof(kTitle)) - 1;
    int title_x_center = w / 2;
    int title_y = h / 3;
    draw_text_luma(
        pi, DrawParams{.px = title_x_center, .py = title_y, .scale = kTitleScale, .value = kFgY},
        std::string_view(kTitle, static_cast<size_t>(title_chars)));

    // Device-path subtitle: font is uppercase-only, so upper-case the
    // string before drawing. Truncate to fit canvas width at scale 2.
    if (device_path && *device_path) {
        int sub_scale = 2;
        int char_w = font8x8::kCharW * sub_scale;
        int max_chars = (w - 32) / char_w;
        std::string_view dp_view(device_path);
        char sub[256];
        int n = 0;
        for (; n < static_cast<int>(dp_view.size()) && n < int(sizeof(sub)) - 1 && n < max_chars;
             ++n) {
            char c = dp_view[n];
            if (c >= 'a' && c <= 'z')
                c = char(c - 'a' + 'A');
            sub[n] = c;
        }
        sub[n] = 0;
        int sub_y = title_y + font8x8::kCharH * kTitleScale + 24;
        draw_text_luma(pi, DrawParams{.px = w / 2, .py = sub_y, .scale = sub_scale, .value = kFgY},
                       std::string_view(sub, static_cast<size_t>(n)));
    }
    return true;
}

bool paint_tick(std::span<uint8_t> nv12, int w, int h, uint64_t tick_idx, uint64_t wallclock_ms,
                const char* status) {
    const std::size_t need = static_cast<std::size_t>(w) * static_cast<std::size_t>(h) * 3 / 2;
    if (w <= 0 || h <= 0 || nv12.size() < need)
        return false;

    const size_t y_size = static_cast<size_t>(w) * h;
    std::span<uint8_t> y_span = nv12.subspan(0, y_size);
    PlaneInfo pi{.plane = y_span, .w = w, .h = h};

    AnimRegion r = derive_anim_region(w, h);

    // Clear a strip extending a bit above r.y_start to include the
    // status-line row (paint_tick is allowed to write there too).
    int status_top = r.y_start - (font8x8::kCharH * kTickScale) - 8;
    if (status_top < 0)
        status_top = 0;
    fill_luma(pi, 0, status_top, w, r.y_end, kBgY);

    if (status && *status) {
        std::string_view sv(status);
        int n = static_cast<int>(std::min(sv.size(), size_t(64)));
        draw_text_luma(
            pi, DrawParams{.px = w / 2, .py = status_top + 4, .scale = kTickScale, .value = kFgY},
            sv.substr(0, n));
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

    int text_y = r.y_start + 4;
    if (n > 0) {
        draw_text_luma(pi,
                       DrawParams{.px = w / 2, .py = text_y, .scale = kTickScale, .value = kFgY},
                       std::string_view(buf, static_cast<size_t>(n)));
    }

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
        fill_luma(pi, dot_x - 3, dot_y - 3, dot_x + 3, dot_y + 3, v);
    }
    return true;
}

} // namespace placeholder_painter
