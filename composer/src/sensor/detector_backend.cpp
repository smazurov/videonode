#include "src/sensor/detector_backend.hpp"

#include "src/common/log_levels.hpp"

#include <algorithm>
#include <array>
#include <cerrno>
#include <csignal>
#include <cstdio>
#include <cstdlib>
#include <cstring>
#include <poll.h>
#include <string_view>
#include <sys/prctl.h>
#include <sys/socket.h>
#include <sys/types.h>
#include <sys/wait.h>
#include <unistd.h>

extern char** environ;

namespace sensor {

namespace {

constexpr uint32_t kMaxFrameBytes = 64U * 1024U * 1024U;

bool set_sndbuf(int fd, int bytes) {
    return ::setsockopt(fd, SOL_SOCKET, SO_SNDBUF, &bytes, sizeof(bytes)) == 0;
}

bool write_full(int fd, std::span<const uint8_t> buf) {
    while (!buf.empty()) {
        ssize_t n = ::write(fd, buf.data(), buf.size());
        if (n < 0) {
            if (errno == EINTR)
                continue;
            return false;
        }
        if (n == 0)
            return false;
        buf = buf.subspan(static_cast<size_t>(n));
    }
    return true;
}

bool writable_now(int fd) {
    pollfd pfd{.fd = fd, .events = POLLOUT, .revents = 0};
    return ::poll(&pfd, 1, 0) > 0 && (pfd.revents & POLLOUT) != 0;
}

std::optional<Detection> parse_line(std::string_view line) {
    Detection d;
    std::array<char, 16> kind{};
    float conf = 0.0F;
    unsigned seq = 0;
    int matched = std::sscanf(std::string(line).c_str(), "%u %15s %f %f %f %f %f", &seq,
                              kind.data(), &conf, &d.x, &d.y, &d.w, &d.h);
    if (matched < 3)
        return std::nullopt;
    d.seq = seq;
    d.kind = kind.data();
    d.confidence = conf;
    return d;
}

} // namespace

DetectorBackend::~DetectorBackend() {
    stop();
}

bool DetectorBackend::spawn_(const std::string& shell_cmd, int width, int height) {
    std::array<int, 2> sv{-1, -1};
    if (::socketpair(AF_UNIX, SOCK_STREAM | SOCK_CLOEXEC, 0, sv.data()) != 0) {
        vn::log::error("sensor: socketpair: %s", std::strerror(errno));
        return false;
    }
    vn::base::unique_fd host(sv[0]);
    vn::base::unique_fd child(sv[1]);

    std::string w_val = std::to_string(width);
    std::string h_val = std::to_string(height);
    std::array<char*, 4> argv{const_cast<char*>("/bin/sh"), const_cast<char*>("-c"),
                              const_cast<char*>(shell_cmd.c_str()), nullptr};

    pid_t pid = ::fork();
    if (pid < 0) {
        vn::log::error("sensor: fork: %s", std::strerror(errno));
        return false;
    }
    if (pid == 0) {
        ::prctl(PR_SET_PDEATHSIG, SIGKILL);
        ::setpgid(0, 0);
        const int cfd = child.get();
        if (::dup2(cfd, STDIN_FILENO) < 0 || ::dup2(cfd, STDOUT_FILENO) < 0)
            ::_exit(127);
        ::setenv("VN_WIDTH", w_val.c_str(), 1);
        ::setenv("VN_HEIGHT", h_val.c_str(), 1);
        ::execve("/bin/sh", argv.data(), environ);
        ::_exit(127);
    }

    pid_ = pid;
    if (!set_sndbuf(host.get(), static_cast<int>(4 * std::max(1, width * height))))
        vn::log::warn("sensor: SO_SNDBUF bump failed: %s", std::strerror(errno));
    sock_ = std::move(host);
    return true;
}

bool DetectorBackend::start(const std::string& shell_cmd, int width, int height) {
    if (width <= 0 || height <= 0)
        return false;
    return spawn_(shell_cmd, width, height);
}

bool DetectorBackend::submit(uint32_t seq, std::span<const uint8_t> y_plane) {
    if (sock_.get() < 0 || y_plane.empty() || y_plane.size() > kMaxFrameBytes)
        return false;
    std::lock_guard<std::mutex> g(write_mu_);
    if (!writable_now(sock_.get()))
        return false;
    std::array<uint32_t, 2> hdr{seq, static_cast<uint32_t>(y_plane.size())};
    auto hdr_bytes = std::as_bytes(std::span(hdr));
    if (!write_full(sock_.get(), std::span(reinterpret_cast<const uint8_t*>(hdr_bytes.data()),
                                           hdr_bytes.size()))) {
        sock_.reset();
        return false;
    }
    if (!write_full(sock_.get(), y_plane)) {
        sock_.reset();
        return false;
    }
    return true;
}

std::optional<Detection> DetectorBackend::poll_detection(int timeout_ms) {
    if (sock_.get() < 0)
        return std::nullopt;
    for (;;) {
        for (size_t nl = 0; nl < rx_.size(); ++nl) {
            if (rx_[nl] != '\n')
                continue;
            std::string_view line(reinterpret_cast<const char*>(rx_.data()), nl);
            auto det = parse_line(line);
            rx_.erase(rx_.begin(), rx_.begin() + static_cast<long>(nl) + 1);
            if (det)
                return det;
            return std::nullopt;
        }
        pollfd pfd{.fd = sock_.get(), .events = POLLIN, .revents = 0};
        int pr = ::poll(&pfd, 1, timeout_ms);
        if (pr <= 0)
            return std::nullopt;
        std::array<uint8_t, 512> buf{};
        ssize_t n = ::read(sock_.get(), buf.data(), buf.size());
        if (n <= 0) {
            if (n < 0 && errno == EINTR)
                continue;
            sock_.reset();
            return std::nullopt;
        }
        auto chunk = std::span(buf).first(static_cast<size_t>(n));
        rx_.insert(rx_.end(), chunk.begin(), chunk.end());
        timeout_ms = 0;
    }
}

bool DetectorBackend::alive() const {
    return sock_.get() >= 0;
}

void DetectorBackend::stop() {
    sock_.reset();
    if (pid_ > 0) {
        ::kill(-pid_, SIGKILL);
        int status = 0;
        ::waitpid(pid_, &status, 0);
        pid_ = -1;
    }
}

} // namespace sensor
