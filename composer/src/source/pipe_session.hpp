#pragma once

#include "src/render/nv12_buf.hpp"
#include "src/source/y4m_reader.hpp"

#include <chrono>
#include <string>
#include <sys/types.h>
#include <vector>

namespace source {

// Child-command lifecycle + y4m -> NV12 dma-buf ring for pipe-mode sources.
// Owns spawn / 1 Hz respawn pacing / reap; reports outcomes as Events so the
// orchestrator can drive the health probe and broadcast without V4L2 coupling.
class PipeSession {
  public:
    enum class Event { None, ChildStarted, FormatDetected, Frame, ChildDown };

    void init(nv12_buf::Allocator& alloc, std::string cmd);
    [[nodiscard]] Event tick(std::chrono::steady_clock::time_point now);
    [[nodiscard]] Event consume();
    [[nodiscard]] int fd() const { return fd_; }
    [[nodiscard]] const Y4mFormat& format() const { return reader_.format(); }
    [[nodiscard]] nv12_buf::Buffer& last_frame() { return ring_[last_slot_]; }
    void stop();

  private:
    [[nodiscard]] bool spawn_child_();
    [[nodiscard]] bool ensure_ring_();
    [[nodiscard]] bool commit_frame_();
    void teardown_();

    nv12_buf::Allocator* alloc_ = nullptr;
    std::string cmd_;
    Y4mReader reader_;
    std::vector<nv12_buf::Buffer> ring_;
    int ring_w_ = 0;
    int ring_h_ = 0;
    size_t write_slot_ = 0;
    size_t last_slot_ = 0;
    pid_t pid_ = -1;
    int fd_ = -1;
    std::chrono::steady_clock::time_point next_spawn_attempt_{};
};

} // namespace source
