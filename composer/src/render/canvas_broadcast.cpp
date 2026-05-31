#include "src/render/canvas_broadcast.hpp"

#include "src/common/log_levels.hpp"
#include "src/ipc/dmabuf_header.hpp"
#include "src/ipc/scm_rights_producer.hpp"
#include "src/render/csc.hpp"

#include <chrono>
#include <utility>

namespace render {

namespace {

uint64_t now_ns() {
    using namespace std::chrono;
    return static_cast<uint64_t>(
        duration_cast<nanoseconds>(steady_clock::now().time_since_epoch()).count());
}

// fds/offsets are the staged (CPU-coherent) plane fds the snapshot mmaps —
// on the gbm/Mesa path the raw GPU dma-buf is not coherent for a separate
// read, so the snapshot must use the same staged copy the broadcast sends.
vn::snapshot::FrameRef make_snapshot_ref(const nv12_buf::Buffer& b, uint64_t frame_idx, int y_fd,
                                         int uv_fd, uint32_t y_off, uint32_t uv_off) {
    vn::snapshot::FrameRef r{};
    r.format = vn::snapshot::Format::Nv12;
    r.width = static_cast<uint32_t>(b.width);
    r.height = static_cast<uint32_t>(b.height);
    r.pitch_y = b.y_pitch;
    r.pitch_uv = b.uv_pitch;
    r.planes[0] = {.fd = y_fd,
                   .offset = y_off,
                   .pitch = b.y_pitch,
                   .row_bytes = static_cast<size_t>(b.width),
                   .rows = static_cast<size_t>(b.height)};
    r.planes[1] = {.fd = uv_fd,
                   .offset = uv_off,
                   .pitch = b.uv_pitch,
                   .row_bytes = static_cast<size_t>(b.width),
                   .rows = static_cast<size_t>(b.height) / 2};
    r.frame_idx = frame_idx;
    r.captured_at_ns = now_ns();
    return r;
}

} // namespace

bool CanvasBroadcast::init(gbm_device* gbm, int out_w, int out_h) {
    if (out_w <= 0 || out_h <= 0 || (out_w & 1) || (out_h & 1))
        return false;
    if (!alloc_.init(gbm))
        return false;
    ring_.clear();
    for (int i = 0; i < kRingDepth; ++i) {
        nv12_buf::Buffer b = alloc_.alloc(out_w, out_h);
        if (!b.valid())
            return false;
        ring_.push_back(std::move(b));
    }
    out_w_ = out_w;
    out_h_ = out_h;
    write_idx_ = 0;
    return true;
}

bool CanvasBroadcast::convert_and_broadcast(const CanvasSrc& canvas,
                                            scm_rights_producer::ScmRightsProducer& prod,
                                            uint64_t frame_idx, vn::snapshot::FrameRef& snap) {
    if (ring_.empty())
        return false;
    nv12_buf::Buffer& b = ring_[write_idx_];
    write_idx_ = (write_idx_ + 1) % static_cast<uint32_t>(ring_.size());

    csc::ConvertParams src;
    src.fd = canvas.fd;
    src.fmt = csc::PixelFormat::Bgra;
    src.width = canvas.width;
    src.height = canvas.height;
    src.wstride = static_cast<int>(canvas.stride);

    csc::ConvertParams dst;
    dst.fd = b.y_fd;
    dst.uv_fd = (b.uv_fd != b.y_fd) ? b.uv_fd : -1;
    dst.fmt = csc::PixelFormat::Nv12;
    dst.width = out_w_;
    dst.height = out_h_;
    dst.wstride = static_cast<int>(b.y_pitch);
    dst.uv_wstride = static_cast<int>(b.uv_pitch);
    dst.color_space = csc::ColorSpace::Bt709Limited;

    if (!csc::convert(src, dst))
        return false;
    nv12_buf::stage_for_read(b);

    const int y_fd = (b.staged_y_fd >= 0) ? b.staged_y_fd : b.y_fd;
    const int uv_fd = (b.staged_uv_fd >= 0) ? b.staged_uv_fd : b.uv_fd;
    const uint32_t y_off = (b.staged_y_fd >= 0) ? 0 : b.y_offset;
    const uint32_t uv_off = (b.staged_uv_fd >= 0) ? 0 : b.uv_offset;

    dmabuf_header::Header h;
    h.width = static_cast<uint32_t>(out_w_);
    h.height = static_cast<uint32_t>(out_h_);
    h.format = "NV12";
    h.plane_pitches = {b.y_pitch, b.uv_pitch};
    h.plane_offsets = {y_off, uv_off};
    h.color_matrix = dmabuf_header::ColorMatrix::Bt709;
    h.color_range = dmabuf_header::ColorRange::Limited;
    h.chroma_siting = dmabuf_header::ChromaSiting::Mpeg2;
    h.frame_idx = frame_idx;
    (void)prod.broadcast(h, {y_fd, uv_fd});

    snap = make_snapshot_ref(b, frame_idx, y_fd, uv_fd, y_off, uv_off);
    return true;
}

} // namespace render
