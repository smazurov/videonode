// Host-side stub implementations for librga and librockchip_mpp.
// Every call here logs once to stderr (per function) and returns failure.
//
// This file is only compiled when CMake can't find the real libraries
// (i.e. on non-Rockchip dev machines). On the rig the real .so's win and
// these stubs aren't built.
//
// See third_party/rockchip-stubs/README.md.

#define _GNU_SOURCE
#include <stdatomic.h>
#include <stdio.h>
#include <string.h>

#include "rga/im2d.h"
#include "rockchip/rk_mpi.h"
#include "rockchip/mpp_frame.h"
#include "rockchip/mpp_packet.h"
#include "rockchip/mpp_buffer.h"

/* Per-function once-flag so we don't spam stderr from a render loop that
 * accidentally got loose. atomic_flag is C11 and lock-free everywhere we
 * care about. */
#define LOG_ONCE(tag)                                                          \
    do {                                                                       \
        static atomic_flag _seen = ATOMIC_FLAG_INIT;                           \
        if (!atomic_flag_test_and_set_explicit(&_seen, memory_order_relaxed))  \
            fprintf(stderr,                                                    \
                    "stub: " tag " called on host build (no real Rockchip libs)\n"); \
    } while (0)

/* ---------------------------------------------------------------- librga */

rga_buffer_handle_t importbuffer_fd(int fd, im_handle_param_t* param) {
    (void)fd; (void)param;
    LOG_ONCE("importbuffer_fd");
    return 0;  /* 0 == invalid handle; rga_csc.cpp checks for this */
}

IM_STATUS releasebuffer_handle(rga_buffer_handle_t handle) {
    (void)handle;
    LOG_ONCE("releasebuffer_handle");
    return IM_STATUS_SUCCESS;
}

rga_buffer_t wrapbuffer_handle_t(rga_buffer_handle_t handle, int w, int h,
                                 int wstride, int hstride, int format) {
    LOG_ONCE("wrapbuffer_handle_t");
    rga_buffer_t b;
    memset(&b, 0, sizeof(b));
    b.handle = handle;
    b.width  = w;
    b.height = h;
    b.wstride = wstride;
    b.hstride = hstride;
    b.format = format;
    return b;
}

IM_STATUS imcvtcolor(rga_buffer_t src, rga_buffer_t dst,
                     int sfmt, int dfmt, int mode) {
    (void)src; (void)dst; (void)sfmt; (void)dfmt; (void)mode;
    LOG_ONCE("imcvtcolor");
    return IM_STATUS_FAILED;
}

/* ---------------------------------------------------------- rockchip-mpp */

MPP_RET mpp_create(MppCtx* ctx, MppApi** mpi) {
    (void)ctx; (void)mpi;
    LOG_ONCE("mpp_create");
    return MPP_NOK;
}

MPP_RET mpp_init(MppCtx ctx, MppCtxType type, MppCodingType coding) {
    (void)ctx; (void)type; (void)coding;
    LOG_ONCE("mpp_init");
    return MPP_NOK;
}

MPP_RET mpp_destroy(MppCtx ctx) {
    (void)ctx;
    LOG_ONCE("mpp_destroy");
    return MPP_OK;
}

MPP_RET mpp_packet_init(MppPacket* packet, void* data, size_t size) {
    (void)data; (void)size;
    if (packet) *packet = NULL;
    LOG_ONCE("mpp_packet_init");
    return MPP_NOK;
}

MPP_RET mpp_packet_deinit(MppPacket* packet) {
    if (packet) *packet = NULL;
    LOG_ONCE("mpp_packet_deinit");
    return MPP_OK;
}

MPP_RET mpp_packet_set_eos(MppPacket packet) {
    (void)packet;
    LOG_ONCE("mpp_packet_set_eos");
    return MPP_OK;
}

MPP_RET mpp_packet_set_pts(MppPacket packet, RK_S64 pts) {
    (void)packet; (void)pts;
    LOG_ONCE("mpp_packet_set_pts");
    return MPP_OK;
}

RK_U32 mpp_frame_get_width      (MppFrame f) { (void)f; LOG_ONCE("mpp_frame_get_width");       return 0; }
RK_U32 mpp_frame_get_height     (MppFrame f) { (void)f; LOG_ONCE("mpp_frame_get_height");      return 0; }
RK_U32 mpp_frame_get_hor_stride (MppFrame f) { (void)f; LOG_ONCE("mpp_frame_get_hor_stride");  return 0; }
RK_U32 mpp_frame_get_ver_stride (MppFrame f) { (void)f; LOG_ONCE("mpp_frame_get_ver_stride");  return 0; }
MppBuffer mpp_frame_get_buffer  (MppFrame f) { (void)f; LOG_ONCE("mpp_frame_get_buffer");      return NULL; }
RK_U32 mpp_frame_get_info_change(MppFrame f) { (void)f; LOG_ONCE("mpp_frame_get_info_change"); return 0; }
RK_U32 mpp_frame_get_errinfo    (MppFrame f) { (void)f; LOG_ONCE("mpp_frame_get_errinfo");     return 0; }
RK_U32 mpp_frame_get_discard    (MppFrame f) { (void)f; LOG_ONCE("mpp_frame_get_discard");     return 0; }
MPP_RET mpp_frame_deinit        (MppFrame* f) { if (f) *f = NULL; LOG_ONCE("mpp_frame_deinit"); return MPP_OK; }

int mpp_buffer_get_fd(MppBuffer b) { (void)b; LOG_ONCE("mpp_buffer_get_fd"); return -1; }
