// vn::signal::install_shutdown — single helper that wires SIGINT/SIGTERM to
// flip a process-wide `std::atomic<bool>` to false, and ignores SIGPIPE (and
// any other caller-specified signals).
//
// Single-instance contract: there is exactly one shutdown flag per process.
// Calling install_shutdown a second time rebinds the static pointer to the
// new flag — the previous flag is no longer reachable from the handler.
// The three videonode binaries each call it exactly once from main().
//
// Header-only: the handler captures the flag via a function-local
// `std::atomic<std::atomic<bool>*>`. We store the pointer atomically so the
// install side has a happens-before edge with any concurrent signal delivery
// (`std::signal` is allowed to race with delivery on multithreaded programs;
// the atomic guarantees the handler sees a fully-constructed pointer).
//
// Restrictions on the handler body (per POSIX async-signal-safety rules):
// the only operation we perform is an `atomic<bool>::store` with relaxed
// memory order — that is guaranteed lock-free on all targets we build for
// (x86_64, aarch64) and is async-signal-safe.

#pragma once

#include <atomic>
#include <csignal>
#include <initializer_list>

namespace vn::signal {

namespace detail {

inline std::atomic<std::atomic<bool>*>& shutdown_flag_slot() {
    static std::atomic<std::atomic<bool>*> slot{nullptr};
    return slot;
}

extern "C" inline void shutdown_handler(int /*signo*/) {
    if (auto* flag = shutdown_flag_slot().load(std::memory_order_acquire); flag != nullptr) {
        flag->store(false, std::memory_order_relaxed);
    }
}

} // namespace detail

// install_shutdown registers `shutdown_handler` for each signal in `sigs`,
// SIG_IGN for each signal in `ignored`, and atomically stores `&running`
// in the handler's flag slot. The flag must outlive the process or be
// rebound before the previous instance is destroyed.
inline void install_shutdown(std::atomic<bool>& running,
                             std::initializer_list<int> sigs = {SIGINT, SIGTERM},
                             std::initializer_list<int> ignored = {SIGPIPE}) {
    // Publish the flag before the handler can observe it. release/acquire
    // pair with the load in shutdown_handler.
    detail::shutdown_flag_slot().store(&running, std::memory_order_release);
    for (int s : sigs) {
        std::signal(s, &detail::shutdown_handler);
    }
    for (int s : ignored) {
        std::signal(s, SIG_IGN);
    }
}

} // namespace vn::signal
