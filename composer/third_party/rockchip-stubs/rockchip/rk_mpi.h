// Host-side stub for rockchip-mpp's rk_mpi.h. See ../README.md.
#ifndef ROCKCHIP_STUBS_RK_MPI_H
#define ROCKCHIP_STUBS_RK_MPI_H

#include "rk_type.h"
#include "mpp_err.h"

#ifdef __cplusplus
extern "C" {
#endif

typedef enum {
    MPP_CTX_DEC = 0,
    MPP_CTX_ENC = 1,
} MppCtxType;

typedef enum {
    MPP_VIDEO_CodingUnused = 0,
    MPP_VIDEO_CodingAVC    = 7,
    MPP_VIDEO_CodingMJPEG  = 8,
    MPP_VIDEO_CodingHEVC   = 9,
} MppCodingType;

typedef enum {
    MPP_DEC_SET_OUTPUT_FORMAT     = 0x10001,
    MPP_DEC_SET_PARSER_SPLIT_MODE = 0x10002,
    MPP_DEC_SET_INFO_CHANGE_READY = 0x10003,
} MpiCmd;

typedef enum {
    MPP_PORT_INPUT  = 0,
    MPP_PORT_OUTPUT = 1,
} MppPortType;

typedef enum {
    MPP_POLL_BLOCK     = -1,
    MPP_POLL_NON_BLOCK = 0,
} MppPollType;

typedef void* MppParam;
typedef void* MppTask;

/* MppApi is a vtable of function pointers in real librockchip_mpp. We
 * mirror only the entries the spike actually invokes. */
struct MppApi_t {
    RK_U32 (*size)(void);
    RK_U32 (*version)(void);

    MPP_RET (*decode_put_packet)(MppCtx ctx, MppPacket pkt);
    MPP_RET (*decode_get_frame) (MppCtx ctx, MppFrame* frame);

    MPP_RET (*encode_put_frame) (MppCtx ctx, MppFrame frame);
    MPP_RET (*encode_get_packet)(MppCtx ctx, MppPacket* pkt);
    MPP_RET (*encode)           (MppCtx ctx, MppFrame frame, MppPacket* pkt);

    MPP_RET (*poll)   (MppCtx ctx, MppPortType type, MppPollType timeout);
    MPP_RET (*dequeue)(MppCtx ctx, MppPortType type, MppTask* task);
    MPP_RET (*enqueue)(MppCtx ctx, MppPortType type, MppTask task);

    MPP_RET (*control)(MppCtx ctx, MpiCmd cmd, MppParam param);
};

MPP_RET mpp_create (MppCtx* ctx, MppApi** mpi);
MPP_RET mpp_init   (MppCtx ctx, MppCtxType type, MppCodingType coding);
MPP_RET mpp_destroy(MppCtx ctx);

#ifdef __cplusplus
}
#endif

#endif
