// canvas_broadcast — converts the composer's BGRA canvas to NV12 (BT.709
// limited / MPEG-2 siting) and broadcasts it over SCM_RIGHTS. Owns a
// depth-N NV12 ring written round-robin; depth (not refcounting) covers
// consumer read latency before a slot is overwritten.

#pragma once

#include "src/render/nv12_buf.hpp"
#include "src/snapshot/snapshot.hpp"

#include <cstdint>
#include <vector>

struct gbm_device;

namespace scm_rights_producer {
class ScmRightsProducer;
}

namespace render {

class CanvasBroadcast {
  public:
    // gbm: null on the rig (dma_heap backend), the csc_placebo gbm device on
    // Mesa. out_w/out_h default to the canvas dims (downscale = pass smaller).
    [[nodiscard]] bool init(gbm_device* gbm, int out_w, int out_h);

    // CSC the BGRA canvas dma-buf into the next ring slot and broadcast it.
    // Fills `snap` with a FrameRef for the written slot (raw fds, no pin).
    [[nodiscard]] bool convert_and_broadcast(int canvas_fd, int canvas_w, int canvas_h,
                                             uint32_t canvas_stride,
                                             scm_rights_producer::ScmRightsProducer& prod,
                                             uint64_t frame_idx, vn::snapshot::FrameRef& snap);

    [[nodiscard]] int out_w() const { return out_w_; }
    [[nodiscard]] int out_h() const { return out_h_; }

  private:
    static constexpr int kRingDepth = 4;
    nv12_buf::Allocator alloc_;
    std::vector<nv12_buf::Buffer> ring_;
    uint32_t write_idx_ = 0;
    int out_w_ = 0;
    int out_h_ = 0;
};

} // namespace render
