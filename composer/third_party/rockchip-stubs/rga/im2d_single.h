// Host-side stub for librga's im2d_single.h. See ../README.md.
#ifndef ROCKCHIP_STUBS_IM2D_SINGLE_H
#define ROCKCHIP_STUBS_IM2D_SINGLE_H

#include "im2d_type.h"

#ifdef __cplusplus
extern "C" {
#endif

/* Subset the spike calls. Real librga has many more im* functions. */
IM_STATUS imcvtcolor(rga_buffer_t src, rga_buffer_t dst,
                     int sfmt, int dfmt, int mode);

#ifdef __cplusplus
}
#endif

#endif
