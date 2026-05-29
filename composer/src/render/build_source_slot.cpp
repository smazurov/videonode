#include "src/render/build_source_slot.hpp"

#include <algorithm>
#include <cstring>

namespace render {

namespace {

// Native (pre-rotation) crop window in normalized source UV space.
struct CropWindow {
    float x0 = 0.0F;
    float x1 = 1.0F;
    float y0 = 0.0F;
    float y1 = 1.0F;
};

// Map a crop window expressed in DISPLAY space (vis_w/vis_h are the visible
// fractions of the displayed image; crop_x/crop_y pan the displayed image,
// 0.5 = centred) onto the source-native axes that pl_compose samples.
//
// pl_compose applies the rotation to the SOURCE frame (PL_ROTATION is
// clockwise; image->rotation = PL_ROTATION_90 rotates the image 90° to the
// right) while the destination crop stays axis-aligned. So for 90°/270° the
// displayed width maps to the native HEIGHT axis and vice-versa, and the pan
// direction follows the clockwise corner mapping:
//   90°  CW: display(p,q) ↔ native(a=q,   b=1-p)
//   270° CW: display(p,q) ↔ native(a=1-q, b=p)
//   180°:    display(p,q) ↔ native(a=1-p, b=1-q)
CropWindow rotated_crop_window(int rotation, float vis_w, float vis_h, float crop_x, float crop_y) {
    auto nvis_x = vis_w;
    auto nvis_y = vis_h;
    auto pan_x = crop_x;
    auto pan_y = crop_y;
    switch (((rotation % 360) + 360) % 360) {
    case 90:
        nvis_x = vis_h;
        pan_x = crop_y;
        nvis_y = vis_w;
        pan_y = 1.0F - crop_x;
        break;
    case 180:
        pan_x = 1.0F - crop_x;
        pan_y = 1.0F - crop_y;
        break;
    case 270:
        nvis_x = vis_h;
        pan_x = 1.0F - crop_y;
        nvis_y = vis_w;
        pan_y = crop_x;
        break;
    default:
        break;
    }
    CropWindow w;
    w.x0 = pan_x * (1.0F - nvis_x);
    w.x1 = w.x0 + nvis_x;
    w.y0 = pan_y * (1.0F - nvis_y);
    w.y1 = w.y0 + nvis_y;
    return w;
}

} // namespace

pl_compose::SourceSlot build_source_slot(const FrameGeom& frame, const LayoutRect& rect,
                                         const SourceState* state) {
    pl_compose::SourceSlot s;
    s.src_y_fd = frame.y_fd;
    s.src_uv_fd = frame.uv_fd;
    s.src_w = frame.width;
    s.src_h = frame.height;
    s.src_y_pitch = frame.y_pitch ? frame.y_pitch : frame.width;
    s.src_uv_pitch = frame.uv_pitch ? frame.uv_pitch : frame.width;
    s.src_y_offset = frame.y_offset;
    s.src_uv_offset = frame.uv_offset;
    s.x = rect.x;
    s.y = rect.y;
    s.w = rect.w;
    s.h = rect.h;
    s.rotation = rect.rotation;

    if (rect.aspect_ratio_mode != 0 && frame.width > 0 && frame.height > 0 && rect.w > 0 &&
        rect.h > 0) {
        auto src_ar = static_cast<float>(frame.width) / static_cast<float>(frame.height);
        auto slot_ar = static_cast<float>(rect.w) / static_cast<float>(rect.h);
        // pl_compose rotates the source, so the on-canvas aspect ratio is the
        // rotated one (h/w for 90°/270°). All AR math runs against disp_ar.
        auto disp_ar = (rect.rotation % 180 != 0) ? 1.0F / src_ar : src_ar;
        if (rect.aspect_ratio_mode == 1) {
            // Fit: letterbox/pillarbox — shrink destination to match displayed AR
            if (disp_ar > slot_ar) {
                auto new_h = static_cast<int>(static_cast<float>(rect.w) / disp_ar);
                s.y += (rect.h - new_h) / 2;
                s.h = new_h;
            } else {
                auto new_w = static_cast<int>(static_cast<float>(rect.h) * disp_ar);
                s.x += (rect.w - new_w) / 2;
                s.w = new_w;
            }
        } else if (rect.aspect_ratio_mode == 2) {
            // Crop: fill slot, position crop window via crop_x/crop_y/crop_scale.
            // vis_* are visible fractions in DISPLAY space; map to native axes.
            auto vis_w = std::min(1.0F, slot_ar / disp_ar);
            auto vis_h = std::min(1.0F, disp_ar / slot_ar);
            if (rect.crop_scale > 1.0F) {
                vis_w = std::min(1.0F, vis_w / rect.crop_scale);
                vis_h = std::min(1.0F, vis_h / rect.crop_scale);
            }
            auto win = rotated_crop_window(rect.rotation, vis_w, vis_h, rect.crop_x, rect.crop_y);
            s.src_crop_x0 = win.x0;
            s.src_crop_x1 = win.x1;
            s.src_crop_y0 = win.y0;
            s.src_crop_y1 = win.y1;
        }
    }

    if (state != nullptr && state->state != "placeholder" && state->has_perspective)
        std::memcpy(s.warp.m, state->warp.data(), 9 * sizeof(float));

    return s;
}

} // namespace render
