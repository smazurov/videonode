#include "src/ipc/scm_rights_producer.hpp"

#include "src/common/log_levels.hpp"
#include "src/common/unique_fd.hpp"
#include "src/ipc/scm_socket.hpp"

#include <cerrno>
#include <cstring>
#include <fcntl.h>
#include <poll.h>
#include <sys/socket.h>
#include <unistd.h>
#include <utility>

namespace scm_rights_producer {

using vn::base::unique_fd;

namespace {

// Set O_NONBLOCK on a connected socket. The producer never wants to block
// a per-consumer send — slow consumers drop frames, fast ones don't pay.
bool set_nonblock(int fd) {
    int flags = ::fcntl(fd, F_GETFL, 0);
    if (flags < 0)
        return false;
    return ::fcntl(fd, F_SETFL, flags | O_NONBLOCK) == 0;
}

} // namespace

bool ScmRightsProducer::init(const InitParams& p) {
    params_ = p;
    if (params_.socket_path.empty()) {
        vn::log::error("scm_rights_producer: socket_path is required");
        return false;
    }
    if (params_.max_consumers <= 0)
        params_.max_consumers = 16;

    // Backlog of max_consumers gives us headroom for bursts of dials.
    listen_fd_ = scm_socket::BindAndListen(params_.socket_path, params_.max_consumers);
    if (!listen_fd_) {
        vn::log::error("scm_rights_producer: BindAndListen(%s): %s", params_.socket_path.c_str(),
                       strerror(errno));
        return false;
    }
    vn::log::info("scm_rights_producer: listening on %s (max_consumers=%d)",
                  params_.socket_path.c_str(), params_.max_consumers);
    return true;
}

bool ScmRightsProducer::start() {
    if (!listen_fd_) {
        vn::log::error("scm_rights_producer: start() before init()");
        return false;
    }
    running_.store(true);
    accept_thread_ = std::thread([this] { this->accept_loop_(); });
    return true;
}

void ScmRightsProducer::stop() {
    stop_requested_.store(true);
    if (listen_fd_) {
        // Shutdown wakes the accept() in the loop thread.
        ::shutdown(listen_fd_.get(), SHUT_RDWR);
    }
    if (accept_thread_.joinable())
        accept_thread_.join();
    listen_fd_.reset();

    {
        std::lock_guard<std::mutex> g(consumers_mu_);
        consumers_.clear(); // ~Consumer closes each fd
    }
    if (!params_.socket_path.empty()) {
        ::unlink(params_.socket_path.c_str());
    }
    running_.store(false);
}

ScmRightsProducer::~ScmRightsProducer() {
    stop();
}

void ScmRightsProducer::accept_loop_() {
    while (!stop_requested_.load()) {
        pollfd pfd{.fd = listen_fd_.get(), .events = POLLIN, .revents = 0};
        int r = ::poll(&pfd, 1, 250);
        if (r < 0) {
            if (errno == EINTR)
                continue;
            // listen_fd was shut down or invalid — exit cleanly.
            return;
        }
        if (r == 0)
            continue;
        if (!(pfd.revents & POLLIN))
            continue;

        unique_fd cfd = scm_socket::AcceptOne(listen_fd_.get());
        if (!cfd) {
            if (errno == EBADF || errno == EINVAL)
                return; // we're shutting down
            continue;
        }

        std::lock_guard<std::mutex> g(consumers_mu_);
        if (static_cast<int>(consumers_.size()) >= params_.max_consumers) {
            vn::log::warn("scm_rights_producer: max_consumers (%d) reached; closing dial",
                          params_.max_consumers);
            // cfd closes via destructor when it goes out of scope.
            continue;
        }
        if (!set_nonblock(cfd.get())) {
            vn::log::error("scm_rights_producer: set_nonblock failed: %s", strerror(errno));
            continue;
        }
        int cfd_raw = cfd.get();
        consumers_.push_back(Consumer{.fd = std::move(cfd), .frames_sent = 0, .frames_dropped = 0});
        vn::log::info("scm_rights_producer: consumer connected (fd=%d, total=%zu)", cfd_raw,
                      consumers_.size());
    }
}

bool ScmRightsProducer::broadcast(const dmabuf_header::Header& header,
                                  const std::vector<int>& fds) {
    std::lock_guard<std::mutex> g(consumers_mu_);
    ++frame_counter_;
    if (consumers_.empty())
        return false;

    std::vector<size_t> to_evict;
    to_evict.reserve(consumers_.size());
    for (size_t i = 0; i < consumers_.size(); ++i) {
        auto& c = consumers_[i];
        // SendMessage is blocking by default; we set O_NONBLOCK on accept,
        // so a full buffer surfaces as EAGAIN/EWOULDBLOCK and we record a
        // drop rather than stalling the producer.
        bool ok = scm_socket::SendMessage(c.fd.get(), header, fds);
        if (ok) {
            ++c.frames_sent;
            continue;
        }
        if (errno == EAGAIN || errno == EWOULDBLOCK) {
            ++c.frames_dropped;
            continue;
        }
        // EPIPE / ECONNRESET / EBADF — consumer is gone. Evict.
        vn::log::info("scm_rights_producer: consumer fd=%d gone (%s); evicting", c.fd.get(),
                      strerror(errno));
        evicted_.push_back(ConsumerStats{.fd = c.fd.get(),
                                         .frames_sent = c.frames_sent,
                                         .frames_dropped = c.frames_dropped,
                                         .evicted_at_frame = frame_counter_});
        to_evict.push_back(i);
    }

    // Erase evicted (highest index first to keep remaining indices valid).
    // ~Consumer closes each fd.
    for (auto it = to_evict.rbegin(); it != to_evict.rend(); ++it) {
        consumers_.erase(consumers_.begin() + *it);
    }
    return true;
}

int ScmRightsProducer::prune_dead_consumers() {
    std::lock_guard<std::mutex> g(consumers_mu_);
    if (consumers_.empty())
        return 0;
    std::vector<pollfd> pfds;
    pfds.reserve(consumers_.size());
    for (const auto& c : consumers_) {
        pfds.push_back(pollfd{.fd = c.fd.get(), .events = 0, .revents = 0});
    }
    // 0ms timeout: just sample current state. POLLHUP/POLLERR/POLLNVAL are
    // always reported regardless of events mask.
    if (::poll(pfds.data(), pfds.size(), 0) <= 0)
        return 0;
    std::vector<size_t> to_evict;
    to_evict.reserve(consumers_.size());
    for (size_t i = 0; i < pfds.size(); ++i) {
        if (pfds[i].revents & (POLLHUP | POLLERR | POLLNVAL)) {
            auto& c = consumers_[i];
            vn::log::info("scm_rights_producer: consumer fd=%d gone (prune); evicting", c.fd.get());
            evicted_.push_back(ConsumerStats{.fd = c.fd.get(),
                                             .frames_sent = c.frames_sent,
                                             .frames_dropped = c.frames_dropped,
                                             .evicted_at_frame = frame_counter_});
            to_evict.push_back(i);
        }
    }
    for (auto it = to_evict.rbegin(); it != to_evict.rend(); ++it) {
        consumers_.erase(consumers_.begin() + *it);
    }
    return static_cast<int>(to_evict.size());
}

int ScmRightsProducer::consumer_count() const {
    std::lock_guard<std::mutex> g(consumers_mu_);
    return static_cast<int>(consumers_.size());
}

std::vector<ConsumerStats> ScmRightsProducer::stats() const {
    std::lock_guard<std::mutex> g(consumers_mu_);
    std::vector<ConsumerStats> out;
    out.reserve(consumers_.size() + evicted_.size());
    for (const auto& c : consumers_) {
        out.push_back(ConsumerStats{.fd = c.fd.get(),
                                    .frames_sent = c.frames_sent,
                                    .frames_dropped = c.frames_dropped,
                                    .evicted_at_frame = 0});
    }
    for (const auto& e : evicted_)
        out.push_back(e);
    return out;
}

} // namespace scm_rights_producer
