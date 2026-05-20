// Host-side stub for rockchip-mpp's mpp_packet.h. See ../README.md.
#ifndef ROCKCHIP_STUBS_MPP_PACKET_H
#define ROCKCHIP_STUBS_MPP_PACKET_H

#include "rk_type.h"
#include "mpp_err.h"

#include <stddef.h>

#ifdef __cplusplus
extern "C" {
#endif

MPP_RET mpp_packet_init     (MppPacket* packet, void* data, size_t size);
MPP_RET mpp_packet_deinit   (MppPacket* packet);
MPP_RET mpp_packet_set_eos  (MppPacket packet);
MPP_RET mpp_packet_set_pts  (MppPacket packet, RK_S64 pts);

#ifdef __cplusplus
}
#endif

#endif
