// Host-side stub for librga's im2d_buffer.h. See ../README.md.
#ifndef ROCKCHIP_STUBS_IM2D_BUFFER_H
#define ROCKCHIP_STUBS_IM2D_BUFFER_H

#include "im2d_type.h"

#ifdef __cplusplus
extern "C" {
#endif

rga_buffer_handle_t importbuffer_fd(int fd, im_handle_param_t* param);
IM_STATUS           releasebuffer_handle(rga_buffer_handle_t handle);
rga_buffer_t        wrapbuffer_handle_t(rga_buffer_handle_t handle,
                                        int width, int height,
                                        int wstride, int hstride, int format);

#ifdef __cplusplus
}
#endif

#endif
