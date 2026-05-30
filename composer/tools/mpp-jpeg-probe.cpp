// mpp-jpeg-probe — validate the Rockchip MPP MJPEG ADVANCED decode API.
//
// MJPEG is the one codec MPP drives through the advanced (1-in-1-out) model
// instead of the simple put-packet/wait-for-info_change/get-frame loop: the
// caller allocates the output frame buffer, attaches it to the input packet
// meta via KEY_OUTPUT_FRAME, puts the packet, and gets exactly that frame
// back. (See ffmpeg-rockchip rkmppdec.c rkmpp_mjpeg_* and mpi_dec_test.c
// dec_advanced, which sets simple=0 only for MPP_VIDEO_CodingMJPEG.)
//
// Usage: ./mpp-jpeg-probe <file.jpg> [W H]   (W,H default 3840 2160)

#if __has_include(<rockchip/rk_mpi.h>)

#include <rockchip/rk_mpi.h>
#include <rockchip/mpp_frame.h>
#include <rockchip/mpp_packet.h>
#include <rockchip/mpp_buffer.h>
#include <rockchip/mpp_meta.h>
#include <rockchip/mpp_log.h>

#include <cerrno>
#include <cstdint>
#include <cstdio>
#include <cstdlib>
#include <cstring>
#include <vector>

#define FFALIGN(x, a) (((x) + (a) - 1) & ~((a) - 1))

namespace {
std::vector<uint8_t> read_file(const char* path) {
    std::vector<uint8_t> data;
    FILE* f = fopen(path, "rb");
    if (!f) {
        fprintf(stderr, "FAIL open %s: %s\n", path, strerror(errno));
        return data;
    }
    fseek(f, 0, SEEK_END);
    long n = ftell(f);
    fseek(f, 0, SEEK_SET);
    if (n > 0) {
        data.resize(static_cast<size_t>(n));
        data.resize(fread(data.data(), 1, data.size(), f));
    }
    fclose(f);
    return data;
}
} // namespace

int main(int argc, char** argv) {
    if (argc < 2) {
        fprintf(stderr, "usage: %s <file.jpg> [W H]\n", argv[0]);
        return 2;
    }
    const char* path = argv[1];
    RK_U32 w = (argc > 2) ? static_cast<RK_U32>(atoi(argv[2])) : 3840;
    RK_U32 h = (argc > 3) ? static_cast<RK_U32>(atoi(argv[3])) : 2160;
    int loops = (argc > 4) ? atoi(argv[4]) : 1;

    std::vector<uint8_t> jpeg = read_file(path);
    if (jpeg.empty())
        return 1;
    printf("input: %s bytes=%zu  hint=%ux%u\n", path, jpeg.size(), w, h);

    // Test: drop MPP's WARN/INFO chatter (e.g. per-frame "mpp_buf_slot:
    // mismatch size_total") while keeping FATAL/ERROR.
    if (getenv("QUIET")) {
        mpp_set_log_level(MPP_LOG_ERROR);
        printf("mpp_set_log_level(ERROR), now=%d\n", mpp_get_log_level());
    }

    MppCtx ctx = nullptr;
    MppApi* mpi = nullptr;
    MPP_RET ret = mpp_create(&ctx, &mpi);
    printf("mpp_create ret=%d\n", ret);
    if (ret != MPP_OK)
        return 1;

    ret = mpp_init(ctx, MPP_CTX_DEC, MPP_VIDEO_CodingMJPEG);
    printf("mpp_init ret=%d\n", ret);
    if (ret != MPP_OK)
        return 1;

    // Internal DRM buffer group for both the input bitstream and the output
    // frame (HALF_INTERNAL model, mirroring rkmppdec.c misc group).
    MppBufferGroup grp = nullptr;
    ret = mpp_buffer_group_get_internal(&grp, MPP_BUFFER_TYPE_DRM | MPP_BUFFER_FLAGS_DMA32 |
                                                  MPP_BUFFER_FLAGS_CACHABLE);
    printf("buffer_group_get_internal(DRM) ret=%d grp=%p\n", ret, grp);
    if (ret != MPP_OK || !grp)
        return 1;

    RK_S64 timeout = 500; // ms, matches production watchdog
    mpi->control(ctx, MPP_SET_OUTPUT_TIMEOUT, &timeout);

    // Mirror production exactly, incl. the ping-pong: hold one prior frame.
    MppFrame held = nullptr;
    for (int iter = 0; iter < loops; ++iter) {
        MppBuffer in_buf = nullptr;
        ret = mpp_buffer_get(grp, &in_buf, jpeg.size());
        if (ret != MPP_OK || !in_buf) {
            printf("STALL: in buffer_get ret=%d at iter %d\n", ret, iter);
            return 1;
        }
        mpp_buffer_write(in_buf, 0, jpeg.data(), jpeg.size());

        MppPacket pkt = nullptr;
        ret = mpp_packet_init_with_buffer(&pkt, in_buf);
        if (ret != MPP_OK || !pkt)
            return 1;
        mpp_packet_set_length(pkt, jpeg.size());

        size_t osz = static_cast<size_t>(FFALIGN(w, 16)) * FFALIGN(h, 16) * 4;
        MppBuffer out_buf = nullptr;
        ret = mpp_buffer_get(grp, &out_buf, osz);
        if (ret != MPP_OK || !out_buf) {
            printf("STALL: out buffer_get ret=%d at iter %d\n", ret, iter);
            return 1;
        }
        MppFrame frame = nullptr;
        mpp_frame_init(&frame);
        mpp_frame_set_buffer(frame, out_buf);
        mpp_buffer_put(out_buf); // PRODUCTION pattern under test
        mpp_meta_set_frame(mpp_packet_get_meta(pkt), KEY_OUTPUT_FRAME, frame);

        ret = mpi->decode_put_packet(ctx, pkt);
        MppFrame out = nullptr;
        if (ret == MPP_OK)
            ret = mpi->decode_get_frame(ctx, &out);

        mpp_packet_deinit(&pkt);
        mpp_buffer_put(in_buf);

        if (ret != MPP_OK || !out) {
            printf("STALL: no frame ret=%d at iter %d\n", ret, iter);
            mpp_frame_deinit(&frame);
            return 1;
        }
        // ping-pong: release the frame we held last round, keep this one
        if (held)
            mpp_frame_deinit(&held);
        held = out;
        if (iter % 200 == 0)
            printf("iter %d ok fd=%d\n", iter, mpp_buffer_get_fd(mpp_frame_get_buffer(out)));
    }
    if (held)
        mpp_frame_deinit(&held);
    printf("RESULT: completed %d iters, no stall\n", loops);
    return 0;
}

#else // <rockchip/rk_mpi.h> ships only on Rockchip; this probe is rig-only.

#include <cstdio>

int main() {
    fprintf(stderr, "mpp-jpeg-probe: requires Rockchip MPP headers (rig-only build)\n");
    return 1;
}

#endif
