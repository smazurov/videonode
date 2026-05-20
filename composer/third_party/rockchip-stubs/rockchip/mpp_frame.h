// Host-side stub for rockchip-mpp's mpp_frame.h. See ../README.md.
#ifndef ROCKCHIP_STUBS_MPP_FRAME_H
#define ROCKCHIP_STUBS_MPP_FRAME_H

#include "rk_type.h"
#include "mpp_err.h"

#ifdef __cplusplus
extern "C" {
#endif

typedef enum {
    MPP_FMT_YUV420SP = 0x00000000,    /* NV12 */
    MPP_FMT_YUV422SP = 0x00000002,
    MPP_FMT_YUV420P  = 0x00000001,
} MppFrameFormat;

RK_U32   mpp_frame_get_width      (MppFrame frame);
RK_U32   mpp_frame_get_height     (MppFrame frame);
RK_U32   mpp_frame_get_hor_stride (MppFrame frame);
RK_U32   mpp_frame_get_ver_stride (MppFrame frame);
MppBuffer mpp_frame_get_buffer    (MppFrame frame);
RK_U32   mpp_frame_get_info_change(MppFrame frame);
RK_U32   mpp_frame_get_errinfo    (MppFrame frame);
RK_U32   mpp_frame_get_discard    (MppFrame frame);
MPP_RET  mpp_frame_deinit         (MppFrame* frame);

#ifdef __cplusplus
}
#endif

#endif
