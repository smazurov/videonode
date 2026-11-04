#include "src/sink/status_reporter.hpp"

#include "src/sink/status_report.hpp"

#include <cerrno>
#include <chrono>
#include <cstring>
#include <span>
#include <sys/socket.h>
#include <sys/time.h>
#include <sys/un.h>
#include <utility>

namespace sink {

StatusReporter::StatusReporter(Params p) : p_(std::move(p)) {}

StatusReporter::~StatusReporter() {
    stop();
}

void StatusReporter::start() {
    std::lock_guard<std::mutex> lk(mu_);
    if (thread_.joinable() || p_.socket_path.empty())
        return;
    stop_ = false;
    thread_ = std::thread([this] { thread_main_(); });
}

void StatusReporter::stop() {
    {
        std::lock_guard<std::mutex> lk(mu_);
        stop_ = true;
    }
    cv_.notify_all();
    if (thread_.joinable())
        thread_.join();
    disconnect_();
}

void StatusReporter::publish(std::string block) {
    {
        std::lock_guard<std::mutex> lk(mu_);
        pending_ = std::move(block);
    }
    cv_.notify_one();
}

void StatusReporter::disconnect_() {
    fd_.reset();
    connected_.store(false);
}

bool StatusReporter::ensure_connected_() {
    if (fd_)
        return true;
    sockaddr_un addr{};
    addr.sun_family = AF_UNIX;
    std::span<char> path(addr.sun_path);
    if (p_.socket_path.size() >= path.size())
        return false;
    std::memcpy(path.data(), p_.socket_path.c_str(), p_.socket_path.size() + 1);

    vn::base::unique_fd fd(::socket(AF_UNIX, SOCK_STREAM | SOCK_CLOEXEC, 0));
    if (!fd)
        return false;
    const timeval tv{.tv_sec = p_.send_timeout_ms / 1000,
                     .tv_usec = static_cast<suseconds_t>((p_.send_timeout_ms % 1000) * 1000)};
    (void)::setsockopt(fd.get(), SOL_SOCKET, SO_SNDTIMEO, &tv, sizeof(tv));
    if (::connect(fd.get(), reinterpret_cast<const sockaddr*>(&addr), sizeof(addr)) != 0)
        return false;
    fd_ = std::move(fd);
    if (!send_all_(kStatusHello)) {
        disconnect_();
        return false;
    }
    connected_.store(true);
    return true;
}

bool StatusReporter::send_all_(std::string_view data) {
    while (!data.empty()) {
        const ssize_t n = ::send(fd_.get(), data.data(), data.size(), MSG_NOSIGNAL);
        if (n < 0 && errno == EINTR)
            continue;
        if (n <= 0)
            return false;
        data.remove_prefix(static_cast<size_t>(n));
    }
    return true;
}

void StatusReporter::thread_main_() {
    std::unique_lock<std::mutex> lk(mu_);
    while (!stop_) {
        cv_.wait(lk, [this] { return stop_ || pending_.has_value(); });
        if (stop_)
            break;
        std::string block = std::move(*pending_);
        pending_.reset();
        lk.unlock();
        const bool sent = ensure_connected_() && send_all_(block);
        if (!sent)
            disconnect_();
        lk.lock();
        if (sent || stop_)
            continue;
        if (!pending_)
            pending_ = std::move(block);
        cv_.wait_for(lk, std::chrono::nanoseconds(p_.retry_ns), [this] { return stop_; });
    }
}

} // namespace sink
