#include "src/source/pipe_session.hpp"

#include "src/common/log_levels.hpp"
#include "src/process/child_process.hpp"

#include <cerrno>
#include <csignal>
#include <cstring>
#include <fcntl.h>
#include <sys/wait.h>

namespace source {

namespace {

constexpr size_t kRingDepth = 3;
constexpr int kReapGracefulMs = 500;

void copy_i420_to_nv12(std::span<const uint8_t> tight, const nv12_buf::Mapping& m, int w, int h) {
    const size_t luma = size_t(w) * size_t(h);
    const size_t chroma = luma / 4;
    auto src_y = tight.first(luma);
    auto src_u = tight.subspan(luma, chroma);
    auto src_v = tight.subspan(luma + chroma, chroma);
    auto dst_y = m.y_bytes();
    auto dst_uv = m.uv_bytes();
    for (int y = 0; y < h; ++y) {
        std::memcpy(dst_y.subspan(size_t(y) * m.y_pitch, size_t(w)).data(),
                    src_y.subspan(size_t(y) * size_t(w), size_t(w)).data(), size_t(w));
    }
    const size_t half_w = size_t(w) / 2;
    for (int y = 0; y < h / 2; ++y) {
        auto u_row = src_u.subspan(size_t(y) * half_w, half_w);
        auto v_row = src_v.subspan(size_t(y) * half_w, half_w);
        auto uv_row = dst_uv.subspan(size_t(y) * m.uv_pitch, size_t(w));
        for (size_t x = 0; x < half_w; ++x) {
            uv_row[2 * x] = u_row[x];
            uv_row[2 * x + 1] = v_row[x];
        }
    }
}

} // namespace

void PipeSession::init(nv12_buf::Allocator& alloc, std::string cmd) {
    alloc_ = &alloc;
    cmd_ = std::move(cmd);
}

PipeSession::Event PipeSession::tick(std::chrono::steady_clock::time_point now) {
    if (pid_ > 0) {
        int status = 0;
        if (::waitpid(pid_, &status, WNOHANG) == pid_) {
            vn::log::warn("videonode-source: pipe child exited (status=%d), respawning", status);
            teardown_();
            return Event::ChildDown;
        }
        return Event::None;
    }
    if (now < next_spawn_attempt_)
        return Event::None;
    next_spawn_attempt_ = now + std::chrono::seconds(1);
    if (!spawn_child_())
        return Event::None;
    return Event::ChildStarted;
}

PipeSession::Event PipeSession::consume() {
    if (fd_ < 0)
        return Event::None;
    switch (reader_.consume_fd(fd_)) {
    case Y4mReader::Result::Header:
        if (!ensure_ring_()) {
            teardown_();
            return Event::ChildDown;
        }
        return Event::FormatDetected;
    case Y4mReader::Result::Frame:
        if (!commit_frame_())
            return Event::None;
        return Event::Frame;
    case Y4mReader::Result::NeedMore:
        return Event::None;
    case Y4mReader::Result::Eof:
        vn::log::info("videonode-source: pipe stdout EOF, respawning");
        teardown_();
        return Event::ChildDown;
    case Y4mReader::Result::Error:
        vn::log::error("videonode-source: pipe stream error: %s", reader_.error().c_str());
        teardown_();
        return Event::ChildDown;
    }
    return Event::None;
}

bool PipeSession::spawn_child_() {
    auto r = child_process::spawn_shell_group(cmd_, SIGKILL);
    if (r.pid <= 0)
        return false;
    if (::fcntl(r.stdout_fd, F_SETFL, O_NONBLOCK) < 0) {
        vn::log::error("child_process: fcntl(O_NONBLOCK): %s", strerror(errno));
        child_process::reap_group(r.pid, r.stdout_fd, kReapGracefulMs);
        return false;
    }
    pid_ = r.pid;
    fd_ = r.stdout_fd;
    reader_.reset();
    return true;
}

bool PipeSession::ensure_ring_() {
    const Y4mFormat& f = reader_.format();
    vn::log::info("videonode-source: pipe format detected %dx%d@%u", f.width, f.height, f.fps());
    if (!ring_.empty() && ring_w_ == f.width && ring_h_ == f.height)
        return true;
    ring_.clear();
    for (size_t i = 0; i < kRingDepth; ++i) {
        nv12_buf::Buffer b = alloc_->alloc(f.width, f.height);
        if (!b.valid()) {
            vn::log::error("videonode-source: pipe ring alloc failed (%dx%d)", f.width, f.height);
            ring_.clear();
            return false;
        }
        ring_.push_back(std::move(b));
    }
    ring_w_ = f.width;
    ring_h_ = f.height;
    write_slot_ = 0;
    last_slot_ = 0;
    return true;
}

bool PipeSession::commit_frame_() {
    nv12_buf::Buffer& dst = ring_[write_slot_];
    auto m = nv12_buf::map_rw(dst);
    if (!m.y || !m.uv)
        return false;
    copy_i420_to_nv12(reader_.frame(), m, ring_w_, ring_h_);
    nv12_buf::unmap(dst);
    nv12_buf::stage_for_read(dst);
    last_slot_ = write_slot_;
    write_slot_ = (write_slot_ + 1) % ring_.size();
    return true;
}

void PipeSession::teardown_() {
    child_process::reap_group(pid_, fd_, kReapGracefulMs);
    pid_ = -1;
    fd_ = -1;
    reader_.reset();
}

void PipeSession::stop() {
    teardown_();
    ring_.clear();
    ring_w_ = 0;
    ring_h_ = 0;
}

} // namespace source
