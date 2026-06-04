// Raise the soft RLIMIT_NOFILE to the hard limit.
//
// Go 1.19+ restores the original (low) soft limit for child processes,
// so binaries spawned by the Go daemon typically inherit soft=1024 even
// when the hard limit is 65536. GPU compositors easily exceed 1024 fds
// (DRM render nodes, GBM BOs, EGL images, gRPC + SCM sockets).

#pragma once

#include <sys/resource.h>

namespace vn {

inline void raise_fd_limit() {
    struct rlimit rl{};
    if (getrlimit(RLIMIT_NOFILE, &rl) == 0 && rl.rlim_cur < rl.rlim_max) {
        rl.rlim_cur = rl.rlim_max;
        setrlimit(RLIMIT_NOFILE, &rl);
    }
}

} // namespace vn
