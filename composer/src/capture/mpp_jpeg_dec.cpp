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

#include <cstdlib>
#include <cstring>

namespace mpp_jpeg_dec {

namespace {

// Watchdog ceiling (milliseconds) for decode_get_frame. NOT a measured decode
// time — an upper bound so a wedged VDPU can't hang the capture thread. MJPEG
// is 1-in-1-out, so the frame is ready right after put_packet and get_frame
// returns far under this in the normal path.
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

// Both buffer groups: DRM dma-bufs, DMA32-addressable, CPU-cachable (we write
// the input bitstream via the CPU and sync before the VDPU reads). Matches the
// ffmpeg-rockchip rkmppdec misc group.
constexpr int kBufFlags = MPP_BUFFER_TYPE_DRM | MPP_BUFFER_FLAGS_DMA32 | MPP_BUFFER_FLAGS_CACHABLE;

// MPP library log level: quiet by default, raised when VN_MPP_LOG is set so the
// hal's own buf_slot / parser / info_change diagnostics surface on the rig
// without a rebuild (mirrors ffmpeg's FFMPEG_RKMPP_DEC_OPT env hook).
int env_mpp_log_level() {
    const char* v = std::getenv("VN_MPP_LOG");
    if (v == nullptr || *v == '\0')
        return MPP_LOG_FATAL;
    return (*v == 'v' || *v == 'V' || *v == '2') ? MPP_LOG_VERBOSE : MPP_LOG_INFO;
}

// Human-readable chroma subsampling the hal decoded into, for diagnostics.
const char* mpp_fmt_name(MppFrameFormat fmt) {
    switch (fmt & MPP_FRAME_FMT_MASK) {
    case MPP_FMT_YUV420SP:
        return "YUV420(NV12)";
    case MPP_FMT_YUV422SP:
        return "YUV422(NV16)";
    case MPP_FMT_YUV444SP:
        return "YUV444(NV24)";
    default:
        return "unsupported";
    }
}

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
MppFrameFormat FrameRef::fmt() const {
    return frame_ ? mpp_frame_get_fmt(frame_) : MPP_FMT_YUV420SP;
}

void MppJpegDec::teardown_() {
    pending_.reset(); // release held frame back to buf_group_ before tearing it down
    if (ctx_ != nullptr) {
        mpp_destroy(ctx_);
        ctx_ = nullptr;
    }
    mpi_ = nullptr;
    if (buf_group_ != nullptr) {
        mpp_buffer_group_put(buf_group_);
        buf_group_ = nullptr;
    }
    if (buf_group_misc_ != nullptr) {
        mpp_buffer_group_put(buf_group_misc_);
        buf_group_misc_ = nullptr;
    }
}

bool MppJpegDec::init(int max_width, int max_height) {
    max_w_ = max_width;
    max_h_ = max_height;

    mpp_set_log_level(env_mpp_log_level());

    MppCtx ctx = nullptr;
    MppApi* mpi = nullptr;
    MPP_RET ret = mpp_create(&ctx, &mpi);
    if (ret != MPP_OK || ctx == nullptr || mpi == nullptr) {
        vn::log::error("mpp_jpeg_dec: mpp_create=%d", ret);
        return false;
    }
    ctx_ = ctx;
    mpi_ = mpi;

    ret = mpp_init(ctx, MPP_CTX_DEC, MPP_VIDEO_CodingMJPEG);
    if (ret != MPP_OK) {
        vn::log::error("mpp_jpeg_dec: mpp_init=%d", ret);
        teardown_();
        return false;
    }

    // Two groups, mirroring rkmppdec.c: buf_group_misc_ holds the input
    // bitstream (and the first, pre-handshake output frame); buf_group_ holds
    // real output frames once attached via SET_EXT_BUF_GROUP on frame one.
    if (mpp_buffer_group_get_internal(&buf_group_misc_, kBufFlags) != MPP_OK ||
        buf_group_misc_ == nullptr) {
        vn::log::error("mpp_jpeg_dec: buffer_group_get_internal(misc) failed");
        teardown_();
        return false;
    }
    if (mpp_buffer_group_get_internal(&buf_group_, kBufFlags) != MPP_OK || buf_group_ == nullptr) {
        vn::log::error("mpp_jpeg_dec: buffer_group_get_internal(out) failed");
        teardown_();
        return false;
    }

    // Disable the IEP deinterlacer: our MJPEG is progressive. Warn-only — the
    // flag is moot for baseline JPEG, so a build that rejects it shouldn't fail
    // init.
    int deint = 0;
    MPP_RET deint_ret = mpi->control(ctx, MPP_DEC_SET_ENABLE_DEINTERLACE, &deint);
    if (deint_ret != MPP_OK)
        vn::log::warn("mpp_jpeg_dec: SET_ENABLE_DEINTERLACE=%d", deint_ret);

    // Watchdog on get_frame so a wedged VDPU can't hang the capture thread.
    RK_S64 out_timeout = kOutputTimeoutMs;
    MPP_RET to_ret = mpi->control(ctx, MPP_SET_OUTPUT_TIMEOUT, &out_timeout);
    if (to_ret != MPP_OK)
        vn::log::warn("mpp_jpeg_dec: SET_OUTPUT_TIMEOUT=%d", to_ret);

    cfg_done_ = true;
    vn::log::info("mpp_jpeg_dec: init OK %dx%d out_buf=%zu deint=%d timeout_ms=%lld", max_w_,
                  max_h_,
                  static_cast<size_t>(align_up(max_w_)) * align_up(max_h_) * kOutputBytesPerPxX4,
                  deint, static_cast<long long>(kOutputTimeoutMs));
    return true;
}

MppJpegDec::~MppJpegDec() {
    teardown_();
}

MppPacket MppJpegDec::alloc_input_packet_(std::span<const uint8_t> jpeg) {
    // The bitstream must live in a DRM/DMA buffer for the MJPEG hal — a plain
    // malloc'd pointer (mpp_packet_init) is silently never decoded.
    MppBuffer in_buf = nullptr;
    if (mpp_buffer_get(buf_group_misc_, &in_buf, jpeg.size()) != MPP_OK || in_buf == nullptr) {
        vn::log::error("mpp_jpeg_dec: in buffer_get(%zu) failed", jpeg.size());
        return nullptr;
    }
    mpp_buffer_write(in_buf, 0, const_cast<uint8_t*>(jpeg.data()), jpeg.size());
    // Flush the CPU write to DRAM before the VDPU reads it; the buffer is
    // cachable. Mirrors mpi_dec_test / ffmpeg-rkmpp.
    mpp_buffer_sync_partial_end(in_buf, 0, jpeg.size());

    MppPacket pkt = nullptr;
    MPP_RET ret = mpp_packet_init_with_buffer(&pkt, in_buf);
    mpp_buffer_put(in_buf); // pkt holds the ref on success; releases on failure
    if (ret != MPP_OK || pkt == nullptr) {
        vn::log::error("mpp_jpeg_dec: packet_init_with_buffer=%d", ret);
        return nullptr;
    }
    mpp_packet_set_length(pkt, jpeg.size());
    mpp_packet_set_pts(pkt, static_cast<RK_S64>(pts_++));
    return pkt;
}

MppFrame MppJpegDec::alloc_output_frame_(MppPacket pkt) {
    // Before the handshake attaches buf_group_, the decoder only knows about
    // buf_group_misc_, so the first output frame must come from there.
    MppBufferGroup grp = info_change_done_ ? buf_group_ : buf_group_misc_;
    const size_t osz =
        static_cast<size_t>(align_up(max_w_)) * align_up(max_h_) * kOutputBytesPerPxX4;
    MppBuffer out_buf = nullptr;
    if (mpp_buffer_get(grp, &out_buf, osz) != MPP_OK || out_buf == nullptr) {
        vn::log::error("mpp_jpeg_dec: out buffer_get(%zu) failed", osz);
        return nullptr;
    }
    MppFrame frame = nullptr;
    mpp_frame_init(&frame);
    mpp_frame_set_buffer(frame, out_buf); // frame takes its own ref
    mpp_buffer_put(out_buf);              // drop ours; frame keeps it alive
    mpp_meta_set_frame(mpp_packet_get_meta(pkt), KEY_OUTPUT_FRAME, frame);
    return frame;
}

void MppJpegDec::drain_input_packet_(MppFrame out) {
    // After a successful put_packet, MPP owns the input packet and stashes it
    // in the output frame's meta. Free it here (rkmppdec.c does the same) —
    // this replaces the caller deiniting its own packet.
    MppMeta meta = mpp_frame_get_meta(out);
    if (meta == nullptr)
        return;
    MppPacket stashed = nullptr;
    if (mpp_meta_get_packet(meta, KEY_INPUT_PACKET, &stashed) == MPP_OK && stashed != nullptr)
        mpp_packet_deinit(&stashed);
}

bool MppJpegDec::handle_info_change_(MppFrame out) {
    vn::log::info("mpp_jpeg_dec: info change %dx%d fmt=%s", mpp_frame_get_width(out),
                  mpp_frame_get_height(out), mpp_fmt_name(mpp_frame_get_fmt(out)));
    MPP_RET r = mpi_->control(ctx_, MPP_DEC_SET_EXT_BUF_GROUP, buf_group_);
    if (r != MPP_OK) {
        vn::log::error("mpp_jpeg_dec: SET_EXT_BUF_GROUP=%d", r);
        return false;
    }
    int fast = 1;
    r = mpi_->control(ctx_, MPP_DEC_SET_PARSER_FAST_MODE, &fast);
    if (r != MPP_OK) {
        vn::log::error("mpp_jpeg_dec: SET_PARSER_FAST_MODE=%d", r);
        return false;
    }
    r = mpi_->control(ctx_, MPP_DEC_SET_INFO_CHANGE_READY, nullptr);
    if (r != MPP_OK) {
        vn::log::error("mpp_jpeg_dec: SET_INFO_CHANGE_READY=%d", r);
        return false;
    }
    info_change_done_ = true;
    return true;
}

FrameRef MppJpegDec::decode(std::span<const uint8_t> jpeg) {
    if (ctx_ == nullptr || mpi_ == nullptr || buf_group_misc_ == nullptr)
        return {};

    MppPacket pkt = alloc_input_packet_(jpeg);
    if (pkt == nullptr)
        return {};
    MppFrame frame = alloc_output_frame_(pkt);
    if (frame == nullptr) {
        mpp_packet_deinit(&pkt); // releases the input buffer
        return {};
    }

    MPP_RET ret = mpi_->decode_put_packet(ctx_, pkt);
    if (ret != MPP_OK) {
        // MPP did not take ownership; we still own both.
        vn::log::error("mpp_jpeg_dec: put_packet=%d", ret);
        mpp_packet_deinit(&pkt);
        mpp_frame_deinit(&frame);
        return {};
    }
    // On success MPP owns pkt + the attached frame; do not free them here.

    MppFrame out = nullptr;
    ret = mpi_->decode_get_frame(ctx_, &out);
    if (ret != MPP_OK || out == nullptr) {
        // Watchdog/wedge path: the attached frame is retained by MPP, nothing
        // safe to free here.
        vn::log::error("mpp_jpeg_dec: get_frame ret=%d out=%s", ret,
                       out == nullptr ? "null" : "ok");
        return {};
    }
    drain_input_packet_(out);

    if (!info_change_done_ && !handle_info_change_(out)) {
        mpp_frame_deinit(&out); // handshake failed; drop, retry next call
        return {};
    }

    RK_U32 err = mpp_frame_get_errinfo(out);
    RK_U32 discard = mpp_frame_get_discard(out);
    if (err != 0 || discard != 0) {
        if (discard_count_ == 0)
            // Escalate the first failure (visible without VN_MPP_LOG). errinfo on
            // a baseline JPEG points at a subsampling the hal rejects (it supports
            // YUV420/422/444).
            vn::log::error("mpp_jpeg_dec: first %s frame %dx%d fmt=%s",
                           err != 0 ? "errinfo" : "discard", mpp_frame_get_width(out),
                           mpp_frame_get_height(out), mpp_fmt_name(mpp_frame_get_fmt(out)));
        else if ((discard_count_ % 150) == 0)
            vn::log::warn("mpp_jpeg_dec: frame err/discard (%llu total)",
                          static_cast<unsigned long long>(discard_count_ + 1));
        ++discard_count_;
        mpp_frame_deinit(&out);
        return {};
    }

    if (mpp_frame_get_buffer(out) == nullptr) {
        // Signaling-only frame (no pixel data); drop. The next packet decodes
        // normally now that the handshake is latched.
        mpp_frame_deinit(&out);
        return {};
    }
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
    out.y_offset = 0;
    // Y plane is hor_stride*ver_stride bytes for every semi-planar layout, so
    // the UV offset is the same regardless of subsampling. Only the chroma
    // pitch and the reported format differ — a 4:2:2 (NV16) / 4:4:4 (NV24)
    // source must NOT be reported as NV12, or the consumer reads the full-height
    // chroma plane with half-height geometry (the red-ghosting bug).
    out.uv_offset = hs * vs;
    switch (f.fmt() & MPP_FRAME_FMT_MASK) {
    case MPP_FMT_YUV422SP:
        out.pixel_format = jpeg_dec::PixelFormat::Nv16;
        out.uv_pitch = hs;
        break;
    case MPP_FMT_YUV444SP:
        out.pixel_format = jpeg_dec::PixelFormat::Nv24;
        out.uv_pitch = 2 * hs;
        break;
    default: // MPP_FMT_YUV420SP
        out.pixel_format = jpeg_dec::PixelFormat::Nv12;
        out.uv_pitch = hs;
        break;
    }
    // Stash the new frame; this drops the previous one (and returns its
    // buffer to the MPP pool). The fd we just exposed in `out` therefore
    // stays valid until the *next* call.
    pending_ = std::move(f);
    return true;
}

} // namespace mpp_jpeg_dec

#endif // HAVE_MPP
