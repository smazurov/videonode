// Host-side stub for librga's im2d_type.h. See ../README.md.
#ifndef ROCKCHIP_STUBS_IM2D_TYPE_H
#define ROCKCHIP_STUBS_IM2D_TYPE_H

#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

typedef uint64_t rga_buffer_handle_t;

typedef struct {
    int            format;
    int            width;
    int            height;
    int            wstride;
    int            hstride;
    rga_buffer_handle_t handle;
    /* real librga has more fields; the spike doesn't access them */
} rga_buffer_t;

typedef struct {
    int x, y, width, height;
} im_rect;

typedef struct {
    int  width;
    int  height;
    int  format;
    /* real librga has more fields; the spike doesn't access them */
} im_handle_param_t;

typedef enum {
    IM_STATUS_NOERROR             = 2,
    IM_STATUS_SUCCESS             = 1,
    IM_STATUS_NOT_SUPPORTED       = -1,
    IM_STATUS_OUT_OF_MEMORY       = -2,
    IM_STATUS_INVALID_PARAM       = -3,
    IM_STATUS_ILLEGAL_PARAM       = -4,
    IM_STATUS_ERROR_VERSION       = -5,
    IM_STATUS_FAILED              = -6,
} IM_STATUS;

/* RK_FORMAT_* values used by composer-spike + videonode-source. Stub
 * values; on the rig the real librga headers from /usr/include override
 * and provide canonical values. */
enum {
    RK_FORMAT_BGRA_8888       = 0x10000,
    RK_FORMAT_RGBA_8888       = 0x10001,
    RK_FORMAT_YCbCr_420_SP    = 0x10002,  /* NV12 */
    RK_FORMAT_YCrCb_420_SP    = 0x10003,  /* NV21 */
    RK_FORMAT_YCbCr_422_SP    = 0x10004,  /* NV16 */
    RK_FORMAT_YCbCr_444_SP    = 0x10005,  /* NV24 */
    RK_FORMAT_BGR_888         = 0x10006,  /* BGR3 packed */
    RK_FORMAT_RGB_888         = 0x10007,
    RK_FORMAT_YUYV_422        = 0x10008,
    RK_FORMAT_UYVY_422        = 0x10009,
};

/* imcvtcolor's color-space mode. */
enum {
    IM_COLOR_SPACE_DEFAULT = 0,
};

#ifdef __cplusplus
}
#endif

#endif
