#pragma once

#include <atomic>
#include <string>

namespace sink {

struct RawSessionArgs {
    std::string socket_path;
    bool verbose = false;
    int poll_ms = 5;
    int settle_ms = 200;
    int first_frame_timeout_s = 30;
};

// Legacy carrier: raw NV12 to stdout, one write per producer frame.
[[nodiscard]] int run_raw_session(const RawSessionArgs& a, std::atomic<bool>& running);

} // namespace sink
