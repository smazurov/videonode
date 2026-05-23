#include "src/capture/mpp_jpeg_dec.hpp"

#include "src/common/log_levels.hpp"

#include <rockchip/rk_mpi.h>
#include <rockchip/mpp_frame.h>
#include <rockchip/mpp_packet.h>
#include <rockchip/mpp_buffer.h>

#include <cstring>
#include <thread>
#include <chrono>

namespace mpp_jpeg_dec {

namespace {

// Wait-poll backoff for decode_get_frame. The MJPEG decoder is fast — most
// 1080p frames decode in well under 5 ms — so a short sleep loop is fine.
constexpr int kMaxGetFrameRetries = 50;
constexpr int kSleepUsPerRetry = 200;

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
    (void)max_width;
    (void)max_height;
    MppCtx ctx = nullptr;
    MppApi* mpi = nullptr;
    MPP_RET ret;

    ret = mpp_create(&ctx, &mpi);
    if (ret != MPP_OK || !ctx || !mpi) {
        vn::log::error("mpp_jpeg_dec: mpp_create=%d", ret);
        return false;
    }
    ctx_ = ctx;
    mpi_ = mpi;

    // Request NV12 output explicitly so we never need a CSC pass after decode.
    MppFrameFormat fmt = MPP_FMT_YUV420SP;
    ret = mpi->control(ctx, MPP_DEC_SET_OUTPUT_FORMAT, &fmt);
    if (ret != MPP_OK) {
        vn::log::warn("mpp_jpeg_dec: SET_OUTPUT_FORMAT=%d", ret);
        // Continue: many MPP builds default to NV12 anyway. Log + go.
    }

    // Tell the decoder to parse headers in-band (no separate extradata channel).
    RK_U32 need_split = 0;
    ret = mpi->control(ctx, MPP_DEC_SET_PARSER_SPLIT_MODE, &need_split);
    if (ret != MPP_OK) {
        vn::log::warn("mpp_jpeg_dec: SET_PARSER_SPLIT_MODE=%d", ret);
    }

    ret = mpp_init(ctx, MPP_CTX_DEC, MPP_VIDEO_CodingMJPEG);
    if (ret != MPP_OK) {
        vn::log::error("mpp_jpeg_dec: mpp_init=%d", ret);
        mpp_destroy(ctx);
        ctx_ = nullptr;
        mpi_ = nullptr;
        return false;
    }
    cfg_done_ = true;
    return true;
}

MppJpegDec::~MppJpegDec() {
    if (ctx_) {
        mpp_destroy(ctx_);
        ctx_ = nullptr;
    }
}

FrameRef MppJpegDec::decode(std::span<const uint8_t> jpeg) {
    if (!ctx_ || !mpi_)
        return {};
    MppCtx c = ctx_;
    MppApi* m = mpi_;

    MppPacket pkt = nullptr;
    MPP_RET ret = mpp_packet_init(&pkt, const_cast<uint8_t*>(jpeg.data()), jpeg.size());
    if (ret != MPP_OK || !pkt) {
        vn::log::error("mpp_jpeg_dec: packet_init=%d", ret);
        return {};
    }
    // Mark end-of-stream so the decoder flushes this single JPEG immediately.
    mpp_packet_set_eos(pkt);

    ret = m->decode_put_packet(c, pkt);
    if (ret != MPP_OK) {
        vn::log::error("mpp_jpeg_dec: put_packet=%d", ret);
        mpp_packet_deinit(&pkt);
        return {};
    }

    MppFrame frame = nullptr;
    bool info_change_handled = false;
    for (int i = 0; i < kMaxGetFrameRetries; ++i) {
        ret = m->decode_get_frame(c, &frame);
        if (ret == MPP_OK && frame) {
            if (mpp_frame_get_info_change(frame)) {
                if (info_change_handled) {
                    vn::log::error("mpp_jpeg_dec: info_change loop?");
                    mpp_frame_deinit(&frame);
                    break;
                }
                m->control(c, MPP_DEC_SET_INFO_CHANGE_READY, nullptr);
                mpp_frame_deinit(&frame);
                frame = nullptr;
                info_change_handled = true;
                continue;
            }
            if (mpp_frame_get_errinfo(frame) || mpp_frame_get_discard(frame)) {
                vn::log::warn("mpp_jpeg_dec: frame err/discard");
                mpp_frame_deinit(&frame);
                frame = nullptr;
                break;
            }
            break;
        }
        std::this_thread::sleep_for(std::chrono::microseconds(kSleepUsPerRetry));
    }

    mpp_packet_deinit(&pkt);
    if (!frame) {
        vn::log::error("mpp_jpeg_dec: no frame after %d retries", kMaxGetFrameRetries);
        return {};
    }
    return FrameRef(frame);
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
