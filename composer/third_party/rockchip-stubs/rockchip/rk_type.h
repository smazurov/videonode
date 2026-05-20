// Host-side stub for rockchip-mpp's rk_type.h. See ../README.md.
#ifndef ROCKCHIP_STUBS_RK_TYPE_H
#define ROCKCHIP_STUBS_RK_TYPE_H

#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

typedef uint8_t  RK_U8;
typedef int8_t   RK_S8;
typedef uint16_t RK_U16;
typedef int16_t  RK_S16;
typedef uint32_t RK_U32;
typedef int32_t  RK_S32;
typedef uint64_t RK_U64;
typedef int64_t  RK_S64;

typedef void* MppCtx;
typedef void* MppFrame;
typedef void* MppPacket;
typedef void* MppBuffer;
typedef void* MppBufferGroup;

/* MppApi is a struct of function pointers in real mpp; we typedef it so
 * pointer types resolve. The struct definition lives in rk_mpi.h. */
struct MppApi_t;
typedef struct MppApi_t MppApi;

#ifdef __cplusplus
}
#endif

#endif
