#include "src/render/build_source_slot.hpp"

#include <algorithm>
#include <cstring>

namespace render {

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
        if (rect.aspect_ratio_mode == 1) {
            // Fit: letterbox/pillarbox — shrink destination to match source AR
            if (src_ar > slot_ar) {
                auto new_h = static_cast<int>(static_cast<float>(rect.w) / src_ar);
                s.y += (rect.h - new_h) / 2;
                s.h = new_h;
            } else {
                auto new_w = static_cast<int>(static_cast<float>(rect.h) * src_ar);
                s.x += (rect.w - new_w) / 2;
                s.w = new_w;
            }
        } else if (rect.aspect_ratio_mode == 2) {
            // Crop: fill slot, position crop window via crop_x/crop_y/crop_scale
            auto vis_w = std::min(1.0F, slot_ar / src_ar);
            auto vis_h = std::min(1.0F, src_ar / slot_ar);
            if (rect.crop_scale > 1.0F) {
                vis_w = std::min(1.0F, vis_w / rect.crop_scale);
                vis_h = std::min(1.0F, vis_h / rect.crop_scale);
            }
            s.src_crop_x0 = rect.crop_x * (1.0F - vis_w);
            s.src_crop_x1 = s.src_crop_x0 + vis_w;
            s.src_crop_y0 = rect.crop_y * (1.0F - vis_h);
            s.src_crop_y1 = s.src_crop_y0 + vis_h;
        }
    }

    if (state != nullptr && state->state != "placeholder" && state->has_perspective)
        std::memcpy(s.warp.m, state->warp.data(), 9 * sizeof(float));

    return s;
}

} // namespace render
