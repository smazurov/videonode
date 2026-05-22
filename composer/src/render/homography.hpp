// homography — corners→3×3 row-major homography solver for the GPU composer.
//
// The shader at gl_compose.cpp does `texCoord = u_warp * vec3(uv, 1)` with a
// homogeneous divide. So the matrix we upload must map **destination UV →
// source UV**: i.e. given the four corners of the *source* quadrilateral
// (where the user clicked on a still frame to identify a tilted screen,
// document, etc.), map them onto the unit square so the output rectangle
// shows the rectified content.
//
// Corner convention matches the rest of the codebase
// (internal/api/models/models.go PerspectiveData):
//
//     [0] = top-left  (TL)
//     [1] = top-right (TR)
//     [2] = bottom-right (BR)
//     [3] = bottom-left  (BL)
//
// Corners are in **snapshot pixel coordinates** — the dimensions of the
// still frame the UI captured when the user marked the corners. We
// normalize by snapshot_w/h before solving, so the resulting matrix
// operates in UV space and remains valid even if the live source later
// switches resolution (e.g. HDMI mode change). This is why snapshot
// dimensions travel with the corner data on the wire.

#pragma once

#include <cstdint>

namespace homography {

// Result codes for corners_to_warp. Caller chooses how to surface failures
// (RPC error, log + skip, etc.).
enum class Status {
    Ok,
    BadSnapshotDims,   // snapshot_w or snapshot_h <= 0
    Degenerate,        // 4 corners are collinear / coincident — no unique homography
};

// Solve the 3×3 row-major homography that maps the unit square
// (0,0),(1,0),(1,1),(0,1) onto the user-supplied corners (normalized by
// snapshot_w/h), so applying `out * vec3(dest_uv, 1)` and dividing by w
// yields the source-uv to sample. Output layout matches GLSL `mat3` with
// `transpose=GL_FALSE` (which gl_compose already uses).
//
// `corners_px` is 8 ints: [TLx, TLy, TRx, TRy, BRx, BRy, BLx, BLy].
[[nodiscard]] Status corners_to_warp(const int corners_px[8],
                                     int snapshot_w,
                                     int snapshot_h,
                                     float out[9]);

} // namespace homography
