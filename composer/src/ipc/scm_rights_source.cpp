#include "src/ipc/scm_rights_source.hpp"

#include "src/common/log_levels.hpp"
#include "src/common/unique_fd.hpp"
#include "src/ipc/dmabuf_header.hpp"
#include "src/ipc/scm_socket.hpp"

#include <cerrno>
#include <chrono>
#include <cstring>
#include <poll.h>
#include <sys/eventfd.h>
#include <sys/socket.h>
#include <thread>
#include <unistd.h>
#include <utility>

namespace scm_rights_source {

using vn::base::unique_fd;

OwnedFrameView& OwnedFrameView::operator=(OwnedFrameView&& o) noexcept {
    if (this == &o)
        return *this;
    fd = std::move(o.fd);
    plane1_fd = std::move(o.plane1_fd);
    width = o.width;
    height = o.height;
    plane0_pitch = o.plane0_pitch;
    plane0_offset = o.plane0_offset;
    plane1_pitch = o.plane1_pitch;
    plane1_offset = o.plane1_offset;
    format = std::move(o.format);
    color_matrix = o.color_matrix;
    frame_idx = o.frame_idx;
    slot_index = o.slot_index;
    generation = o.generation;
    return *this;
}

OwnedFrameView::~OwnedFrameView() = default;

namespace {

// poll so we can periodically check stop_requested instead of blocking accept.
unique_fd wait_and_accept(int listen_fd, std::atomic<bool>& stop, int total_seconds) {
    auto deadline = std::chrono::steady_clock::now() + std::chrono::seconds(total_seconds);
    while (!stop.load() && std::chrono::steady_clock::now() < deadline) {
        pollfd pfd{.fd = listen_fd, .events = POLLIN, .revents = 0};
        int r = ::poll(&pfd, 1, 250);
        if (r < 0) {
            if (errno == EINTR)
                continue;
            return {};
        }
        if (r == 0)
            continue;
        if (pfd.revents & POLLIN) {
            return scm_socket::AcceptOne(listen_fd);
        }
    }
    errno = ETIMEDOUT;
    return {};
}

} // namespace

bool ScmRightsSource::init(const InitParams& p) {
    params_ = p;
    if (params_.socket_path.empty()) {
        vn::log::error("scm_rights_source: socket_path is required");
        return false;
    }
    notify_fd_ = unique_fd(::eventfd(0, EFD_NONBLOCK | EFD_CLOEXEC));
    if (!notify_fd_) {
        vn::log::error("scm_rights_source: eventfd: %s", strerror(errno));
        return false;
    }
    if (params_.dial) {
        return true;
    }
    // Listen mode (legacy): bind+listen in init(); accept is deferred to
    // start() so init() stays non-blocking.
    listen_fd_ = scm_socket::BindAndListen(params_.socket_path, 1);
    if (!listen_fd_) {
        vn::log::error("scm_rights_source: BindAndListen(%s): %s", params_.socket_path.c_str(),
                       strerror(errno));
        return false;
    }
    vn::log::info("scm_rights_source: listening on %s", params_.socket_path.c_str());
    return true;
}

bool ScmRightsSource::start() {
    unique_fd client;
    if (params_.dial) {
        // Retry until the deadline so we don't lose to a race where the
        // producer hasn't bound yet; always attempt at least once.
        const auto deadline =
            std::chrono::steady_clock::now() + std::chrono::milliseconds(params_.dial_timeout_ms);
        while (!stop_requested_.load()) {
            client = scm_socket::ConnectClient(params_.socket_path);
            if (client || std::chrono::steady_clock::now() >= deadline)
                break;
            std::this_thread::sleep_for(std::chrono::milliseconds(100));
        }
        if (!client) {
            vn::log::error("scm_rights_source: dial %s failed: %s", params_.socket_path.c_str(),
                           strerror(errno));
            return false;
        }
        vn::log::info("scm_rights_source: dialed %s", params_.socket_path.c_str());
    } else {
        if (!listen_fd_) {
            vn::log::error("scm_rights_source: start() before init()");
            return false;
        }
        client = wait_and_accept(listen_fd_.get(), stop_requested_, 30);
        if (!client) {
            vn::log::error("scm_rights_source: accept failed: %s", strerror(errno));
            return false;
        }
    }
    client_fd_ = std::move(client);
    running_.store(true);
    thread_ = std::thread([this] { this->thread_main_(); });

    if (params_.dial && !scm_socket::SendReady(client_fd_.get())) {
        vn::log::error("scm_rights_source: SendReady failed: %s", strerror(errno));
        stop();
        return false;
    }
    return true;
}

void ScmRightsSource::stop() {
    stop_requested_.store(true);
    // Shutdown the listen + client sockets to unblock any pending accept
    // (in a concurrent start()) or recvmsg (in our worker). We do NOT
    // call .reset() here — that would race with a concurrent start()
    // reading listen_fd_/client_fd_. The destructor handles fd lifetime;
    // callers that start() from another thread must join() that thread
    // before destroying the ScmRightsSource.
    if (listen_fd_) {
        ::shutdown(listen_fd_.get(), SHUT_RDWR);
    }
    if (client_fd_) {
        ::shutdown(client_fd_.get(), SHUT_RDWR);
    }
    if (thread_.joinable())
        thread_.join();

    // Only unlink the path in listen mode — in dial mode we don't own it.
    if (!params_.dial && !params_.socket_path.empty()) {
        ::unlink(params_.socket_path.c_str());
    }
    {
        std::lock_guard<std::mutex> g(latest_mu_);
        latest_owned_fds_.clear();
        latest_ = {};
    }
    prev_fds_.clear();
    running_.store(false);
}

ScmRightsSource::~ScmRightsSource() {
    stop();
}

void ScmRightsSource::thread_main_() {
    // Clear running_ on every exit path (peer closed, reset, truncation
    // give-up, stop) so a consumer can distinguish a dropped connection from
    // an idle one and re-dial. start() sets running_ true before spawning us.
    struct ClearOnExit {
        std::atomic<bool>& flag;
        ~ClearOnExit() { flag.store(false); }
    } clear_on_exit{running_};

    int consecutive_truncations = 0;
    while (!stop_requested_.load()) {
        dmabuf_header::Header header;
        std::vector<int> fds;
        bool eof = false;
        bool truncated = false;
        if (!scm_socket::RecvMessage(client_fd_.get(), header, fds, &eof, &truncated)) {
            if (eof) {
                vn::log::info("scm_rights_source: peer closed");
                return;
            }
            if (truncated) {
                ++consecutive_truncations;
                if (consecutive_truncations >= 10) {
                    vn::log::error("scm_rights_source: %d consecutive truncations, giving up",
                                   consecutive_truncations);
                    return;
                }
                vn::log::warn("scm_rights_source: frame truncated, skipping (%d consecutive)",
                              consecutive_truncations);
                continue;
            }
            if (!stop_requested_.load()) {
                vn::log::error("scm_rights_source: RecvMessage failed: %s", strerror(errno));
            }
            return;
        }
        consecutive_truncations = 0;

        std::vector<unique_fd> incoming;
        incoming.reserve(fds.size());
        for (int fd : fds)
            incoming.emplace_back(fd);

        FrameView nf;
        nf.width = static_cast<int>(header.width);
        nf.height = static_cast<int>(header.height);
        if (!header.plane_pitches.empty()) {
            nf.plane0_pitch = header.plane_pitches[0];
            nf.plane0_offset = header.plane_offsets[0];
        }
        if (header.plane_pitches.size() >= 2) {
            nf.plane1_pitch = header.plane_pitches[1];
            nf.plane1_offset = header.plane_offsets[1];
        }
        nf.format = header.format;
        nf.color_matrix = header.color_matrix;
        nf.frame_idx = header.frame_idx;
        nf.slot_index = header.slot_index;
        nf.generation = header.generation;
        nf.fd = incoming.size() >= 1 ? incoming[0].get() : -1;
        nf.plane1_fd = incoming.size() >= 2 ? incoming[1].get() : -1;

        std::vector<unique_fd> retired;
        {
            std::lock_guard<std::mutex> g(latest_mu_);
            retired = std::move(latest_owned_fds_);
            latest_owned_fds_ = std::move(incoming);
            latest_ = nf;
        }
        // Now retire the OLD-previous fds (one cycle stale), then move
        // `retired` (just-replaced fds) into prev_fds_. Matches
        // FfmpegPipeSource's ring-buffer pattern at ring_size=2.
        prev_fds_.clear();
        prev_fds_ = std::move(retired);

        if (notify_fd_) {
            uint64_t one = 1;
            (void)::write(notify_fd_.get(), &one, sizeof(one));
        }
    }
}

OwnedFrameView ScmRightsSource::latest_frame() const {
    std::lock_guard<std::mutex> g(latest_mu_);
    OwnedFrameView out;
    if (latest_.fd >= 0)
        out.fd = unique_fd(::dup(latest_.fd));
    if (latest_.plane1_fd >= 0)
        out.plane1_fd = unique_fd(::dup(latest_.plane1_fd));
    out.width = latest_.width;
    out.height = latest_.height;
    out.plane0_pitch = latest_.plane0_pitch;
    out.plane0_offset = latest_.plane0_offset;
    out.plane1_pitch = latest_.plane1_pitch;
    out.plane1_offset = latest_.plane1_offset;
    out.format = latest_.format;
    out.color_matrix = latest_.color_matrix;
    out.frame_idx = latest_.frame_idx;
    out.slot_index = latest_.slot_index;
    out.generation = latest_.generation;
    return out;
}

} // namespace scm_rights_source
