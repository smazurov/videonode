#include "src/sink/raw_session.hpp"

#include "src/common/log_levels.hpp"
#include "src/ipc/scm_rights_source.hpp"
#include "src/sink/nv12_pack.hpp"
#include "src/sink/video_link.hpp"

#include <chrono>
#include <memory>
#include <poll.h>
#include <thread>
#include <unistd.h>

namespace sink {

namespace {

enum class LoopExit : uint8_t {
    SourceDropped,
    StdoutClosed,
    FormatChanged,
    FirstFrameTimeout,
    Shutdown,
};

struct StreamShape {
    bool announced = false;
    int width = 0;
    int height = 0;
};

void wait_for_frame(int notify_fd, int timeout_ms) {
    if (notify_fd < 0) {
        std::this_thread::sleep_for(std::chrono::milliseconds(timeout_ms));
        return;
    }
    pollfd pfd{.fd = notify_fd, .events = POLLIN, .revents = 0};
    (void)::poll(&pfd, 1, timeout_ms);
    if ((pfd.revents & POLLIN) != 0) {
        uint64_t val = 0;
        (void)::read(notify_fd, &val, sizeof(val));
    }
}

[[nodiscard]] LoopExit run_frame_loop(scm_rights_source::ScmRightsSource& src,
                                      const RawSessionArgs& a, StreamShape& shape,
                                      std::atomic<bool>& running, AlignedScratch& scratch) {
    uint64_t last_idx = 0;
    const int nfd = src.notify_fd();
    const auto deadline =
        std::chrono::steady_clock::now() + std::chrono::seconds(a.first_frame_timeout_s);
    while (running.load()) {
        if (!src.running())
            return LoopExit::SourceDropped;
        auto owned = src.latest_frame();
        if (owned.fd.get() < 0 || owned.frame_idx == 0 || owned.frame_idx == last_idx) {
            if (owned.frame_idx == 0 && std::chrono::steady_clock::now() > deadline)
                return LoopExit::FirstFrameTimeout;
            wait_for_frame(nfd, a.poll_ms);
            continue;
        }
        if (shape.announced && (owned.width != shape.width || owned.height != shape.height)) {
            vn::log::warn("videonode-sink: source reconfigured %dx%d -> %dx%d; exiting so "
                          "the encoder rebuilds",
                          shape.width, shape.height, owned.width, owned.height);
            return LoopExit::FormatChanged;
        }
        if (!shape.announced) {
            shape = {.announced = true, .width = owned.width, .height = owned.height};
            vn::log::info("videonode-sink: streaming raw NV12 %dx%d from %s", owned.width,
                          owned.height, a.socket_path.c_str());
        }
        if (a.verbose)
            vn::log::info("videonode-sink: frame_idx=%llu fd=%d",
                          static_cast<unsigned long long>(owned.frame_idx), owned.fd.get());
        last_idx = owned.frame_idx;
        const auto packed = pack_nv12(view_of(owned), scratch);
        if (!packed.empty() && !write_full(STDOUT_FILENO, packed))
            return LoopExit::StdoutClosed;
    }
    return LoopExit::Shutdown;
}

[[nodiscard]] std::unique_ptr<scm_rights_source::ScmRightsSource>
dial_source(const RawSessionArgs& a) {
    auto src = std::make_unique<scm_rights_source::ScmRightsSource>();
    scm_rights_source::InitParams p;
    p.socket_path = a.socket_path;
    p.dial = true;
    if (!src->init(p) || !src->start())
        return nullptr;
    return src;
}

} // namespace

int run_raw_session(const RawSessionArgs& a, std::atomic<bool>& running) {
    StreamShape shape;
    AlignedScratch scratch;
    while (running.load()) {
        auto src = dial_source(a);
        if (!src) {
            if (!shape.announced) {
                vn::log::fatal("videonode-sink: failed to dial %s (is videonode-source up?)",
                               a.socket_path.c_str());
                return 1;
            }
            vn::log::warn("videonode-sink: re-dial %s failed, retrying", a.socket_path.c_str());
            continue;
        }
        std::this_thread::sleep_for(std::chrono::milliseconds(a.settle_ms));

        const LoopExit reason = run_frame_loop(*src, a, shape, running, scratch);
        src->stop();

        if (reason == LoopExit::FirstFrameTimeout && !shape.announced) {
            vn::log::fatal("videonode-sink: timeout waiting for first frame on %s",
                           a.socket_path.c_str());
            return 1;
        }
        if (reason == LoopExit::SourceDropped || reason == LoopExit::FirstFrameTimeout) {
            if (!running.load())
                break;
            vn::log::info("videonode-sink: source gone, reconnecting to %s", a.socket_path.c_str());
            continue;
        }
        if (reason == LoopExit::StdoutClosed)
            vn::log::info("videonode-sink: stdout closed, exiting");
        break;
    }
    return 0;
}

} // namespace sink
