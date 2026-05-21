#include "src/ipc/scm_rights_producer.hpp"

#include "src/ipc/scm_socket.hpp"

#include <cerrno>
#include <chrono>
#include <cstdio>
#include <cstring>
#include <fcntl.h>
#include <poll.h>
#include <sys/socket.h>
#include <sys/un.h>
#include <unistd.h>

namespace scm_rights_producer {

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
        fprintf(stderr, "scm_rights_producer: socket_path is required\n");
        return false;
    }
    if (params_.max_consumers <= 0)
        params_.max_consumers = 16;

    int listen_fd = ::socket(AF_UNIX, SOCK_STREAM | SOCK_CLOEXEC, 0);
    if (listen_fd < 0) {
        fprintf(stderr, "scm_rights_producer: socket: %s\n", strerror(errno));
        return false;
    }
    ::unlink(params_.socket_path.c_str()); // remove stale socket

    sockaddr_un addr{};
    addr.sun_family = AF_UNIX;
    if (params_.socket_path.size() + 1 > sizeof(addr.sun_path)) {
        fprintf(stderr, "scm_rights_producer: socket_path too long\n");
        ::close(listen_fd);
        return false;
    }
    std::memcpy(addr.sun_path, params_.socket_path.c_str(), params_.socket_path.size() + 1);
    if (::bind(listen_fd, reinterpret_cast<sockaddr*>(&addr), sizeof(addr)) < 0) {
        fprintf(stderr, "scm_rights_producer: bind(%s): %s\n", params_.socket_path.c_str(),
                strerror(errno));
        ::close(listen_fd);
        return false;
    }
    // Backlog of max_consumers gives us headroom for bursts of dials.
    if (::listen(listen_fd, params_.max_consumers) < 0) {
        fprintf(stderr, "scm_rights_producer: listen: %s\n", strerror(errno));
        ::close(listen_fd);
        return false;
    }
    listen_fd_ = listen_fd;
    fprintf(stderr, "scm_rights_producer: listening on %s (max_consumers=%d)\n",
            params_.socket_path.c_str(), params_.max_consumers);
    return true;
}

bool ScmRightsProducer::start() {
    if (listen_fd_ < 0) {
        fprintf(stderr, "scm_rights_producer: start() before init()\n");
        return false;
    }
    running_.store(true);
    accept_thread_ = std::thread([this] { this->accept_loop_(); });
    return true;
}

void ScmRightsProducer::stop() {
    stop_requested_.store(true);
    if (listen_fd_ >= 0) {
        // Shutdown wakes the accept() in the loop thread.
        ::shutdown(listen_fd_, SHUT_RDWR);
    }
    if (accept_thread_.joinable())
        accept_thread_.join();
    if (listen_fd_ >= 0) {
        ::close(listen_fd_);
        listen_fd_ = -1;
    }

    std::lock_guard<std::mutex> g(consumers_mu_);
    for (auto& c : consumers_) {
        if (c.fd >= 0)
            ::close(c.fd);
    }
    consumers_.clear();
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
        pollfd pfd{listen_fd_, POLLIN, 0};
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

        int cfd = scm_socket::AcceptOne(listen_fd_);
        if (cfd < 0) {
            if (errno == EBADF || errno == EINVAL)
                return; // we're shutting down
            continue;
        }

        std::lock_guard<std::mutex> g(consumers_mu_);
        if (static_cast<int>(consumers_.size()) >= params_.max_consumers) {
            fprintf(stderr, "scm_rights_producer: max_consumers (%d) reached; closing dial\n",
                    params_.max_consumers);
            ::close(cfd);
            continue;
        }
        if (!set_nonblock(cfd)) {
            fprintf(stderr, "scm_rights_producer: set_nonblock failed: %s\n", strerror(errno));
            ::close(cfd);
            continue;
        }
        consumers_.push_back(Consumer{cfd, 0, 0});
        fprintf(stderr, "scm_rights_producer: consumer connected (fd=%d, total=%zu)\n", cfd,
                consumers_.size());
    }
}

bool ScmRightsProducer::broadcast(const dmabuf_msg::Header& header, const std::vector<int>& fds) {
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
        bool ok = scm_socket::SendMessage(c.fd, header, fds);
        if (ok) {
            ++c.frames_sent;
            continue;
        }
        if (errno == EAGAIN || errno == EWOULDBLOCK) {
            ++c.frames_dropped;
            continue;
        }
        // EPIPE / ECONNRESET / EBADF — consumer is gone. Evict.
        fprintf(stderr, "scm_rights_producer: consumer fd=%d gone (%s); evicting\n", c.fd,
                strerror(errno));
        evicted_.push_back(ConsumerStats{c.fd, c.frames_sent, c.frames_dropped, frame_counter_});
        ::close(c.fd);
        to_evict.push_back(i);
    }

    // Erase evicted (highest index first to keep remaining indices valid).
    for (auto it = to_evict.rbegin(); it != to_evict.rend(); ++it) {
        consumers_.erase(consumers_.begin() + *it);
    }
    return true;
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
        out.push_back(ConsumerStats{c.fd, c.frames_sent, c.frames_dropped, 0});
    }
    for (const auto& e : evicted_)
        out.push_back(e);
    return out;
}

} // namespace scm_rights_producer
