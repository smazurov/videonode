// Host-side stub for rockchip-mpp's mpp_buffer.h. See ../README.md.
#ifndef ROCKCHIP_STUBS_MPP_BUFFER_H
#define ROCKCHIP_STUBS_MPP_BUFFER_H

#include "rk_type.h"
#include "mpp_err.h"

#ifdef __cplusplus
extern "C" {
#endif

int mpp_buffer_get_fd(MppBuffer buffer);

#ifdef __cplusplus
}
#endif

#endif
