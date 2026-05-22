#include "src/render/homography.hpp"

#include <cmath>

namespace homography {

namespace {

// Solve an 8×8 linear system Ax = b in place using Gaussian elimination
// with partial pivoting. Returns false if the matrix is singular (any
// pivot below `eps` after elimination) — that's our "degenerate quad"
// signal.
//
// The standard textbook routine; small enough to inline and keeps us off
// any external linear-algebra dependency. eps is sized for inputs in [0,1]
// after corner normalization; a pivot under 1e-7 means the four source
// corners don't span a quadrilateral (e.g. three are collinear, or two
// coincide).
bool solve_8x8(double A[8][9], double x[8]) {
    constexpr double kPivotEps = 1e-7;
    for (int col = 0; col < 8; ++col) {
        // Partial pivot: find the row >= col with the largest |A[r][col]|
        // and swap it into position `col`.
        int pivot_row = col;
        double pivot_abs = std::fabs(A[col][col]);
        for (int r = col + 1; r < 8; ++r) {
            double a = std::fabs(A[r][col]);
            if (a > pivot_abs) {
                pivot_abs = a;
                pivot_row = r;
            }
        }
        if (pivot_abs < kPivotEps)
            return false;
        if (pivot_row != col) {
            for (int c = col; c < 9; ++c) {
                double tmp = A[col][c];
                A[col][c] = A[pivot_row][c];
                A[pivot_row][c] = tmp;
            }
        }
        // Eliminate rows below.
        for (int r = col + 1; r < 8; ++r) {
            double factor = A[r][col] / A[col][col];
            for (int c = col; c < 9; ++c)
                A[r][c] -= factor * A[col][c];
        }
    }
    // Back-substitute.
    for (int r = 7; r >= 0; --r) {
        double s = A[r][8];
        for (int c = r + 1; c < 8; ++c)
            s -= A[r][c] * x[c];
        x[r] = s / A[r][r];
    }
    return true;
}

} // namespace

Status corners_to_warp(const int corners_px[8],
                       int snapshot_w,
                       int snapshot_h,
                       float out[9]) {
    if (snapshot_w <= 0 || snapshot_h <= 0)
        return Status::BadSnapshotDims;

    // Source corners = unit square (dest UV space we'll be sampling INTO
    // via the shader's u_warp * vec3(dest_uv, 1) / w). Destination corners =
    // user-marked corners normalized to UV [0,1] of the source texture.
    //
    // Build the 8×8 system: for each of 4 (u,v)→(x,y) pairs,
    //   [u v 1 0 0 0 -ux -vx] [h00 h01 h02 h10 h11 h12 h20 h21]^T = x
    //   [0 0 0 u v 1 -uy -vy]                                    = y
    // h22 is fixed at 1.
    const double sw = static_cast<double>(snapshot_w);
    const double sh = static_cast<double>(snapshot_h);
    const double src[4][2] = {
        {0.0, 0.0}, // TL of dest = (0,0)
        {1.0, 0.0}, // TR
        {1.0, 1.0}, // BR
        {0.0, 1.0}, // BL
    };
    const double dst[4][2] = {
        {corners_px[0] / sw, corners_px[1] / sh},
        {corners_px[2] / sw, corners_px[3] / sh},
        {corners_px[4] / sw, corners_px[5] / sh},
        {corners_px[6] / sw, corners_px[7] / sh},
    };

    double A[8][9] = {};
    for (int i = 0; i < 4; ++i) {
        double u = src[i][0];
        double v = src[i][1];
        double x = dst[i][0];
        double y = dst[i][1];
        // Row 2i — solves for x = (h00*u + h01*v + h02) / (h20*u + h21*v + 1).
        A[2 * i][0] = u;
        A[2 * i][1] = v;
        A[2 * i][2] = 1.0;
        A[2 * i][3] = 0.0;
        A[2 * i][4] = 0.0;
        A[2 * i][5] = 0.0;
        A[2 * i][6] = -u * x;
        A[2 * i][7] = -v * x;
        A[2 * i][8] = x;
        // Row 2i+1 — solves for y.
        A[2 * i + 1][0] = 0.0;
        A[2 * i + 1][1] = 0.0;
        A[2 * i + 1][2] = 0.0;
        A[2 * i + 1][3] = u;
        A[2 * i + 1][4] = v;
        A[2 * i + 1][5] = 1.0;
        A[2 * i + 1][6] = -u * y;
        A[2 * i + 1][7] = -v * y;
        A[2 * i + 1][8] = y;
    }

    double h[8] = {};
    if (!solve_8x8(A, h))
        return Status::Degenerate;

    // The 8×8 linear system is solvable as long as the SOURCE points form
    // a quadrilateral (the unit square always does), so pivot failure
    // doesn't catch a degenerate DESTINATION quad — the solver happily
    // produces a singular 3×3. Reject by checking |det(H)| ≈ 0 after the
    // solve. 1e-6 is generous given inputs are normalized to [0,1]; a real
    // homography has |det| O(1).
    constexpr double kDetEps = 1e-6;
    const double a = h[0], b = h[1], c = h[2];
    const double d = h[3], e = h[4], f = h[5];
    const double g = h[6], hh = h[7], i = 1.0;
    const double det = a * (e * i - f * hh) - b * (d * i - f * g) + c * (d * hh - e * g);
    if (std::fabs(det) < kDetEps)
        return Status::Degenerate;

    out[0] = static_cast<float>(h[0]);
    out[1] = static_cast<float>(h[1]);
    out[2] = static_cast<float>(h[2]);
    out[3] = static_cast<float>(h[3]);
    out[4] = static_cast<float>(h[4]);
    out[5] = static_cast<float>(h[5]);
    out[6] = static_cast<float>(h[6]);
    out[7] = static_cast<float>(h[7]);
    out[8] = 1.0f;
    return Status::Ok;
}

} // namespace homography
