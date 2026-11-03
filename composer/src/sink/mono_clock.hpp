#pragma once

#include <cstdint>
#include <ctime>

namespace sink {

[[nodiscard]] inline int64_t mono_ns() {
    timespec ts{};
    ::clock_gettime(CLOCK_MONOTONIC, &ts);
    return static_cast<int64_t>(ts.tv_sec) * 1'000'000'000 + ts.tv_nsec;
}

} // namespace sink
