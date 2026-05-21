#include "src/ipc/scm_rights_source.hpp"

#include "src/rpc/dmabuf_msg.hpp"
#include "src/ipc/scm_socket.hpp"

#include <cerrno>
#include <chrono>
#include <cstdio>
#include <cstring>
#include <poll.h>
#include <sys/socket.h>
#include <sys/un.h>
#include <thread>
#include <unistd.h>

namespace scm_rights_source {

namespace {

void close_all(std::vector<int>& fds) {
    for (int fd : fds) {
        if (fd >= 0)
            ::close(fd);
    }
    fds.clear();
}

// Wait up to total_seconds for `listen_fd` to have an incoming connection.
// Returns the accepted client fd or -1 on timeout/error. We use poll so
// we can periodically check stop_requested.
int wait_and_accept(int listen_fd, std::atomic<bool>& stop, int total_seconds) {
    auto deadline = std::chrono::steady_clock::now() + std::chrono::seconds(total_seconds);
    while (!stop.load() && std::chrono::steady_clock::now() < deadline) {
        pollfd pfd{.fd=listen_fd, .events=POLLIN, .revents=0};
        int r = ::poll(&pfd, 1, 250);
        if (r < 0) {
            if (errno == EINTR)
                continue;
            return -1;
        }
        if (r == 0)
            continue;
        if (pfd.revents & POLLIN) {
            return scm_socket::AcceptOne(listen_fd);
        }
    }
    errno = ETIMEDOUT;
    return -1;
}

} // namespace

bool ScmRightsSource::init(const InitParams& p) {
    params_ = p;
    if (params_.socket_path.empty()) {
        fprintf(stderr, "scm_rights_source: socket_path is required\n");
        return false;
    }
    if (params_.dial) {
        // Dial mode: the producer (sidecar) listens; we connect. start()
        // does the actual connect so init() stays non-blocking.
        return true;
    }
    // Listen mode (legacy): we listen; the daemon dials in. Accept is
    // deferred to start() so init() doesn't block.
    int listen_fd = ::socket(AF_UNIX, SOCK_STREAM | SOCK_CLOEXEC, 0);
    if (listen_fd < 0) {
        fprintf(stderr, "scm_rights_source: socket: %s\n", strerror(errno));
        return false;
    }
    ::unlink(params_.socket_path.c_str()); // remove stale path
    sockaddr_un addr{};
    addr.sun_family = AF_UNIX;
    if (params_.socket_path.size() + 1 > sizeof(addr.sun_path)) {
        fprintf(stderr, "scm_rights_source: socket_path too long\n");
        ::close(listen_fd);
        return false;
    }
    std::memcpy(addr.sun_path, params_.socket_path.c_str(), params_.socket_path.size() + 1);
    if (::bind(listen_fd, reinterpret_cast<sockaddr*>(&addr), sizeof(addr)) < 0) {
        fprintf(stderr, "scm_rights_source: bind(%s): %s\n", params_.socket_path.c_str(),
                strerror(errno));
        ::close(listen_fd);
        return false;
    }
    if (::listen(listen_fd, 1) < 0) {
        fprintf(stderr, "scm_rights_source: listen: %s\n", strerror(errno));
        ::close(listen_fd);
        return false;
    }
    listen_fd_ = listen_fd;
    fprintf(stderr, "scm_rights_source: listening on %s\n", params_.socket_path.c_str());
    return true;
}

bool ScmRightsSource::start() {
    int client = -1;
    if (params_.dial) {
        // Dial the producer's socket. Retry briefly so we don't lose to a
        // race where the producer hasn't bound yet.
        auto deadline = std::chrono::steady_clock::now() + std::chrono::seconds(30);
        while (std::chrono::steady_clock::now() < deadline && !stop_requested_.load()) {
            client = scm_socket::ConnectClient(params_.socket_path);
            if (client >= 0)
                break;
            std::this_thread::sleep_for(std::chrono::milliseconds(100));
        }
        if (client < 0) {
            fprintf(stderr, "scm_rights_source: dial %s failed: %s\n", params_.socket_path.c_str(),
                    strerror(errno));
            return false;
        }
        fprintf(stderr, "scm_rights_source: dialed %s\n", params_.socket_path.c_str());
    } else {
        if (listen_fd_ < 0) {
            fprintf(stderr, "scm_rights_source: start() before init()\n");
            return false;
        }
        client = wait_and_accept(listen_fd_, stop_requested_, 30);
        if (client < 0) {
            fprintf(stderr, "scm_rights_source: accept failed: %s\n", strerror(errno));
            return false;
        }
    }
    client_fd_ = client;
    running_.store(true);
    thread_ = std::thread([this] { this->thread_main_(); });
    return true;
}

void ScmRightsSource::stop() {
    stop_requested_.store(true);
    // Shutdown reads on client to wake any blocked recvmsg.
    if (client_fd_ >= 0) {
        ::shutdown(client_fd_, SHUT_RDWR);
    }
    if (thread_.joinable())
        thread_.join();

    if (client_fd_ >= 0) {
        ::close(client_fd_);
        client_fd_ = -1;
    }
    if (listen_fd_ >= 0) {
        ::close(listen_fd_);
        listen_fd_ = -1;
    }
    // Only unlink the path in listen mode — in dial mode we don't own it.
    if (!params_.dial && !params_.socket_path.empty()) {
        ::unlink(params_.socket_path.c_str());
    }
    {
        std::lock_guard<std::mutex> g(latest_mu_);
        if (latest_.fd >= 0)
            ::close(latest_.fd);
        if (latest_.plane1_fd >= 0)
            ::close(latest_.plane1_fd);
        latest_ = {};
    }
    close_all(prev_fds_);
    running_.store(false);
}

ScmRightsSource::~ScmRightsSource() {
    stop();
}

void ScmRightsSource::thread_main_() {
    while (!stop_requested_.load()) {
        dmabuf_msg::Header header;
        std::vector<int> fds;
        bool eof = false;
        if (!scm_socket::RecvMessage(client_fd_, header, fds, &eof)) {
            if (eof) {
                fprintf(stderr, "scm_rights_source: peer closed\n");
            } else if (!stop_requested_.load()) {
                fprintf(stderr, "scm_rights_source: RecvMessage failed: %s\n", strerror(errno));
            }
            return;
        }

        // We've got a new set of fds. Replace `latest_` and stash the
        // previous one in prev_fds_ to be closed on the next iteration —
        // gives the consumer ~one cycle of validity past the moment we
        // overwrite (matches FfmpegPipeSource's ring-buffer pattern at
        // ring_size=2).
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
        nf.frame_idx = header.frame_idx;
        nf.fd = fds.size() >= 1 ? fds[0] : -1;
        nf.plane1_fd = fds.size() >= 2 ? fds[1] : -1;

        std::vector<int> retired;
        {
            std::lock_guard<std::mutex> g(latest_mu_);
            if (latest_.fd >= 0)
                retired.push_back(latest_.fd);
            if (latest_.plane1_fd >= 0)
                retired.push_back(latest_.plane1_fd);
            latest_ = nf;
        }
        // Now retire the OLD-previous fds (one cycle stale), then move
        // `retired` (just-replaced fds) into prev_fds_.
        close_all(prev_fds_);
        prev_fds_ = std::move(retired);
    }
}

FrameView ScmRightsSource::latest_frame() const {
    std::lock_guard<std::mutex> g(latest_mu_);
    return latest_;
}

} // namespace scm_rights_source
