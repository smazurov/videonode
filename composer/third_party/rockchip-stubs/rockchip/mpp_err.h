// Host-side stub for rockchip-mpp's mpp_err.h. See ../README.md.
#ifndef ROCKCHIP_STUBS_MPP_ERR_H
#define ROCKCHIP_STUBS_MPP_ERR_H

#ifdef __cplusplus
extern "C" {
#endif

typedef enum {
    MPP_OK                 = 0,
    MPP_NOK                = -1,
    MPP_ERR_NULL_PTR       = -2,
    MPP_ERR_MALLOC         = -3,
    MPP_ERR_OPEN_FILE      = -4,
    MPP_ERR_VALUE          = -5,
    MPP_ERR_UNKNOW         = -100,
} MPP_RET;

#ifdef __cplusplus
}
#endif

#endif
