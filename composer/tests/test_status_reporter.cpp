#include "src/sink/status_reporter.hpp"

#include <gtest/gtest.h>

#include <array>
#include <chrono>
#include <cstring>
#include <poll.h>
#include <string>
#include <sys/socket.h>
#include <sys/un.h>
#include <thread>
#include <unistd.h>

namespace {

using std::chrono::milliseconds;
using Clock = std::chrono::steady_clock;

class Listener {
  public:
    explicit Listener(const std::string& path) {
        ::unlink(path.c_str());
        fd_ = ::socket(AF_UNIX, SOCK_STREAM | SOCK_CLOEXEC, 0);
        sockaddr_un addr{};
        addr.sun_family = AF_UNIX;
        std::strncpy(addr.sun_path, path.c_str(), sizeof(addr.sun_path) - 1);
        EXPECT_EQ(::bind(fd_, reinterpret_cast<sockaddr*>(&addr), sizeof(addr)), 0);
        EXPECT_EQ(::listen(fd_, 4), 0);
    }
    ~Listener() { ::close(fd_); }
    Listener(const Listener&) = delete;
    Listener& operator=(const Listener&) = delete;

    [[nodiscard]] int accept_within(milliseconds timeout) const {
        pollfd p{.fd = fd_, .events = POLLIN, .revents = 0};
        if (::poll(&p, 1, static_cast<int>(timeout.count())) <= 0)
            return -1;
        return ::accept4(fd_, nullptr, nullptr, SOCK_CLOEXEC);
    }

  private:
    int fd_ = -1;
};

std::string read_until(int fd, const std::string& needle, milliseconds timeout) {
    std::string got;
    const auto deadline = Clock::now() + timeout;
    std::array<char, 4096> buf{};
    while (got.find(needle) == std::string::npos && Clock::now() < deadline) {
        pollfd p{.fd = fd, .events = POLLIN, .revents = 0};
        if (::poll(&p, 1, 50) <= 0)
            continue;
        const ssize_t n = ::read(fd, buf.data(), buf.size());
        if (n <= 0)
            break;
        got.append(buf.data(), static_cast<size_t>(n));
    }
    return got;
}

std::string temp_socket(const char* name) {
    return "/tmp/vn_status_" + std::to_string(::getpid()) + "_" + name + ".sock";
}

} // namespace

TEST(StatusReporter, SendsHelloThenNewestBlockOnceDaemonAppears) {
    const std::string path = temp_socket("late");
    ::unlink(path.c_str());
    sink::StatusReporter r({.socket_path = path, .retry_ns = 50'000'000});
    r.start();
    r.publish("block=a\n");
    std::this_thread::sleep_for(milliseconds(20));
    r.publish("block=b\n");
    r.publish("block=c\n");
    Listener l(path);
    const int conn = l.accept_within(milliseconds(2000));
    ASSERT_GE(conn, 0);
    const std::string got = read_until(conn, "block=c\n", milliseconds(2000));
    EXPECT_EQ(got, "sink_wire=1\nblock=c\n");
    EXPECT_TRUE(r.connected());
    r.stop();
    ::close(conn);
    ::unlink(path.c_str());
}

TEST(StatusReporter, PublishNeverBlocksWhenDaemonStopsReading) {
    const std::string path = temp_socket("wedged");
    Listener l(path);
    sink::StatusReporter r({.socket_path = path, .retry_ns = 50'000'000, .send_timeout_ms = 100});
    r.start();
    const std::string big(64 * 1024, 'x');
    const auto start = Clock::now();
    for (int i = 0; i < 2000; ++i)
        r.publish(big + "\n");
    EXPECT_LT(Clock::now() - start, milliseconds(500));
    const auto stop_start = Clock::now();
    r.stop();
    EXPECT_LT(Clock::now() - stop_start, milliseconds(1500));
    ::unlink(path.c_str());
}

TEST(StatusReporter, ReconnectsWithFreshHelloAfterDaemonDrops) {
    const std::string path = temp_socket("drop");
    Listener l(path);
    sink::StatusReporter r({.socket_path = path, .retry_ns = 50'000'000});
    r.start();
    r.publish("block=1\n");
    const int first = l.accept_within(milliseconds(2000));
    ASSERT_GE(first, 0);
    EXPECT_EQ(read_until(first, "block=1\n", milliseconds(2000)), "sink_wire=1\nblock=1\n");
    ::close(first);

    int second = -1;
    for (int i = 0; i < 100 && second < 0; ++i) {
        r.publish("block=" + std::to_string(i + 2) + "\n");
        second = l.accept_within(milliseconds(50));
    }
    ASSERT_GE(second, 0);
    const std::string got = read_until(second, "\nblock=", milliseconds(2000));
    EXPECT_EQ(got.rfind("sink_wire=1\n", 0), 0U) << got;
    r.stop();
    ::close(second);
    ::unlink(path.c_str());
}

TEST(StatusReporter, NoSocketPathMeansNoThread) {
    sink::StatusReporter r({.socket_path = ""});
    r.start();
    r.publish("block=x\n");
    EXPECT_FALSE(r.connected());
    r.stop();
}
