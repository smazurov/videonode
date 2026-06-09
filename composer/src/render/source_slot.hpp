#pragma once

namespace pl_compose {

struct Warp {
    float m[9] = {1, 0, 0, 0, 1, 0, 0, 0, 1};
};

struct SourceSlot {
    int src_y_fd = -1;
    int src_uv_fd = -1;
    int src_w = 0;
    int src_h = 0;
    int src_y_pitch = 0;
    int src_uv_pitch = 0;
    int src_y_offset = 0;
    int src_uv_offset = 0;
    int x = 0;
    int y = 0;
    int w = 0;
    int h = 0;
    Warp warp;
    int rotation = 0;
    bool src_bt709 = false;
    float src_crop_x0 = 0.0F;
    float src_crop_y0 = 0.0F;
    float src_crop_x1 = 1.0F;
    float src_crop_y1 = 1.0F;
};

} // namespace pl_compose
