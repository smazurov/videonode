#include "src/capture/mpp_jpeg_dec.hpp"

#include "src/common/log_levels.hpp"

// Mirror of the header guard: this TU is only compiled when HAVE_MPP. The
// preprocessor wrapper keeps clang-tidy / IDE indexers from choking on the
// missing Rockchip headers on dev hosts (where librockchip_mpp is absent
// and the build system excludes this file from the link, but a CDB-less
// probe may still hand it to clang-tidy).
#if defined(HAVE_MPP)

#include <rockchip/rk_mpi.h>
#include <rockchip/mpp_frame.h>
#include <rockchip/mpp_packet.h>
#include <rockchip/mpp_buffer.h>
#include <rockchip/mpp_meta.h>
#include <rockchip/mpp_log.h>

#include <cstring>

namespace mpp_jpeg_dec {

namespace {

// Watchdog ceiling (milliseconds) for decode_get_frame. NOT a measured decode
// time — an upper bound so a wedged VDPU can't hang the capture thread. MJPEG
// is 1-in-1-out (advanced API), so the frame is ready right after put_packet
// and get_frame returns far under this in the normal path.
constexpr RK_S64 kOutputTimeoutMs = 500;

constexpr int kAlign = 16;
constexpr int align_up(int v) {
    return (v + kAlign - 1) & ~(kAlign - 1);
}

// Output frame buffer is over-allocated to 4x aligned(w*h). MPP's buffer-slot
// accounting can size for a 4:2:2 (2 byte/px) layout even when the JPEG is
// 4:2:0; a tight NV12 (1.5x) allocation trips "mpp_buf_slot: mismatch
// size_total" and the decode is rejected. ffmpeg-rockchip uses the same 4x.
constexpr int kOutputBytesPerPxX4 = 4;

} // namespace

void FrameRef::reset() {
    if (frame_) {
        mpp_frame_deinit(&frame_);
        frame_ = nullptr;
    }
}

int FrameRef::width() const {
    return frame_ ? mpp_frame_get_width(frame_) : 0;
}
int FrameRef::height() const {
    return frame_ ? mpp_frame_get_height(frame_) : 0;
}
int FrameRef::hor_stride() const {
    return frame_ ? mpp_frame_get_hor_stride(frame_) : 0;
}
int FrameRef::ver_stride() const {
    return frame_ ? mpp_frame_get_ver_stride(frame_) : 0;
}
int FrameRef::dmabuf_fd() const {
    if (!frame_)
        return -1;
    MppBuffer buf = mpp_frame_get_buffer(frame_);
    if (!buf)
        return -1;
    return mpp_buffer_get_fd(buf);
}

bool MppJpegDec::init(int max_width, int max_height) {
    max_w_ = max_width;
    max_h_ = max_height;
    MppCtx ctx = nullptr;
    MppApi* mpi = nullptr;
    MPP_RET ret;

    // Silence MPP's own per-frame chatter (version banner, "mpp_buf_slot:
    // mismatch size_total", and per-frame "mpp_parser_parse" errors on the
    // occasional bad camera frame). We surface real failures ourselves via
    // return codes + a rate-limited err/discard log, so MPP's redundant
    // stderr would only spam the journal. Process-global, but videonode-source
    // uses MPP solely for this decoder.
    mpp_set_log_level(MPP_LOG_FATAL);

    ret = mpp_create(&ctx, &mpi);
    if (ret != MPP_OK || !ctx || !mpi) {
        vn::log::error("mpp_jpeg_dec: mpp_create=%d", ret);
        return false;
    }
    ctx_ = ctx;
    mpi_ = mpi;

    ret = mpp_init(ctx, MPP_CTX_DEC, MPP_VIDEO_CodingMJPEG);
    if (ret != MPP_OK) {
        vn::log::error("mpp_jpeg_dec: mpp_init=%d", ret);
        mpp_destroy(ctx);
        ctx_ = nullptr;
        mpi_ = nullptr;
        return false;
    }

    // DRM buffer pool for the input bitstream and output NV12 frames. MJPEG is
    // driven through MPP's advanced 1-in-1-out model (it is the only codec for
    // which mpi_dec_test sets simple=0): there is no info_change handshake and
    // no decoder-managed output pool — the caller supplies the output buffer
    // per packet, so we allocate from this group. DMA32 + cachable matches the
    // ffmpeg-rockchip rkmppdec misc group.
    ret = mpp_buffer_group_get_internal(&grp_, MPP_BUFFER_TYPE_DRM | MPP_BUFFER_FLAGS_DMA32 |
                                                   MPP_BUFFER_FLAGS_CACHABLE);
    if (ret != MPP_OK || !grp_) {
        vn::log::error("mpp_jpeg_dec: buffer_group_get_internal=%d", ret);
        mpp_destroy(ctx);
        ctx_ = nullptr;
        mpi_ = nullptr;
        return false;
    }

    // Bound get_frame with a watchdog. In the 1-in-1-out model the frame is
    // ready immediately after put_packet, so this only guards a wedged VDPU.
    RK_S64 out_timeout = kOutputTimeoutMs;
    ret = mpi->control(ctx, MPP_SET_OUTPUT_TIMEOUT, &out_timeout);
    if (ret != MPP_OK) {
        vn::log::warn("mpp_jpeg_dec: SET_OUTPUT_TIMEOUT=%d", ret);
    }

    cfg_done_ = true;
    return true;
}

MppJpegDec::~MppJpegDec() {
    // Release any held frame (returns its buffer to grp_) before tearing down
    // the context and the pool the buffer came from.
    pending_.reset();
    if (ctx_) {
        mpp_destroy(ctx_);
        ctx_ = nullptr;
    }
    if (grp_) {
        mpp_buffer_group_put(grp_);
        grp_ = nullptr;
    }
}

FrameRef MppJpegDec::decode(std::span<const uint8_t> jpeg) {
    if (!ctx_ || !mpi_ || !grp_)
        return {};
    MppCtx c = ctx_;
    MppApi* m = mpi_;

    // Input bitstream must live in a DRM/DMA buffer for the MJPEG hal — a
    // plain malloc'd pointer (mpp_packet_init) is silently never decoded.
    MppBuffer in_buf = nullptr;
    if (mpp_buffer_get(grp_, &in_buf, jpeg.size()) != MPP_OK || !in_buf) {
        vn::log::error("mpp_jpeg_dec: in buffer_get(%zu) failed", jpeg.size());
        return {};
    }
    mpp_buffer_write(in_buf, 0, const_cast<uint8_t*>(jpeg.data()), jpeg.size());
    // Flush the CPU write to DRAM before the VDPU reads it. The input buffer
    // is cachable; without this the hardware can read stale/uninitialised DRAM
    // under memory pressure (the pool hands out a fresh buffer whenever the
    // JPEG size changes), which surfaces as intermittent — and under load,
    // total — "mpp_parser_parse" failures. Mirrors mpi_dec_test / ffmpeg-rkmpp.
    mpp_buffer_sync_partial_end(in_buf, 0, jpeg.size());

    MppPacket pkt = nullptr;
    MPP_RET ret = mpp_packet_init_with_buffer(&pkt, in_buf);
    if (ret != MPP_OK || !pkt) {
        vn::log::error("mpp_jpeg_dec: packet_init_with_buffer=%d", ret);
        mpp_buffer_put(in_buf);
        return {};
    }
    mpp_packet_set_length(pkt, jpeg.size());

    // Caller-supplied output frame buffer (the 1-in-1-out contract).
    const size_t osz =
        static_cast<size_t>(align_up(max_w_)) * align_up(max_h_) * kOutputBytesPerPxX4;
    MppBuffer out_buf = nullptr;
    if (mpp_buffer_get(grp_, &out_buf, osz) != MPP_OK || !out_buf) {
        vn::log::error("mpp_jpeg_dec: out buffer_get(%zu) failed", osz);
        mpp_packet_deinit(&pkt);
        mpp_buffer_put(in_buf);
        return {};
    }
    MppFrame frame = nullptr;
    mpp_frame_init(&frame);
    mpp_frame_set_buffer(frame, out_buf); // frame takes its own ref
    mpp_buffer_put(out_buf);              // drop ours; frame keeps it alive

    mpp_meta_set_frame(mpp_packet_get_meta(pkt), KEY_OUTPUT_FRAME, frame);

    ret = m->decode_put_packet(c, pkt);
    MppFrame out = nullptr;
    if (ret == MPP_OK) {
        ret = m->decode_get_frame(c, &out); // returns the frame we attached
    } else {
        vn::log::error("mpp_jpeg_dec: put_packet=%d", ret);
    }

    mpp_packet_deinit(&pkt);
    mpp_buffer_put(in_buf);

    if (ret != MPP_OK || !out) {
        vn::log::error("mpp_jpeg_dec: no frame (ret=%d)", ret);
        mpp_frame_deinit(&frame);
        return {};
    }
    if (mpp_frame_get_errinfo(out) || mpp_frame_get_discard(out)) {
        // Bad/truncated camera frame — drop it. Rate-limit the log: at 30 fps a
        // burst of corrupt frames would otherwise flood the journal.
        if ((discard_count_++ % 150) == 0)
            vn::log::warn("mpp_jpeg_dec: frame err/discard (%llu total)",
                          static_cast<unsigned long long>(discard_count_));
        mpp_frame_deinit(&out);
        return {};
    }
    // `out` is the same MppFrame object we created and attached; FrameRef now
    // owns it (and through it the output buffer).
    return FrameRef(out);
}

bool MppJpegDec::decode(std::span<const uint8_t> jpeg, jpeg_dec::DecodedNv12& out) {
    FrameRef f = decode(jpeg);
    if (!f.valid())
        return false;
    const uint32_t hs = static_cast<uint32_t>(f.hor_stride());
    const uint32_t vs = static_cast<uint32_t>(f.ver_stride());
    out.fd = f.dmabuf_fd();
    out.width = f.width();
    out.height = f.height();
    out.y_pitch = hs;
    out.uv_pitch = hs;
    out.y_offset = 0;
    out.uv_offset = hs * vs;
    // Stash the new frame; this drops the previous one (and returns its
    // buffer to the MPP pool). The fd we just exposed in `out` therefore
    // stays valid until the *next* call.
    pending_ = std::move(f);
    return true;
}

} // namespace mpp_jpeg_dec

#endif // HAVE_MPP
