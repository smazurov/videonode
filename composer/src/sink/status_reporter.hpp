#pragma once

#include "src/common/unique_fd.hpp"

#include <atomic>
#include <condition_variable>
#include <cstdint>
#include <mutex>
#include <optional>
#include <string>
#include <string_view>
#include <thread>

namespace sink {

// Pushes status blocks to the daemon's per-stream progress socket on its own
// thread. publish() never blocks: only the newest unsent block is kept, and a
// missing or wedged daemon just delays reporting until a reconnect succeeds.
class StatusReporter {
  public:
    struct Params {
        std::string socket_path;
        int64_t retry_ns = 2'000'000'000;
        int send_timeout_ms = 1000;
    };

    explicit StatusReporter(Params p);
    ~StatusReporter();
    StatusReporter(const StatusReporter&) = delete;
    StatusReporter& operator=(const StatusReporter&) = delete;

    void start();
    void stop();
    void publish(std::string block);
    [[nodiscard]] bool connected() const { return connected_.load(); }

  private:
    void thread_main_();
    [[nodiscard]] bool ensure_connected_();
    [[nodiscard]] bool send_all_(std::string_view data);
    void disconnect_();

    Params p_;
    std::thread thread_;
    std::mutex mu_;
    std::condition_variable cv_;
    std::optional<std::string> pending_;
    bool stop_ = false;
    vn::base::unique_fd fd_;
    std::atomic<bool> connected_{false};
};

} // namespace sink
