// Why posix_spawn rather than fork+execvp: posix_spawn is the documented
// async-signal-safe alternative; on Linux glibc backs it with clone+execve
// which is faster (no full COW) and doesn't surprise downstream tooling
// that walks /proc/<pid>/task during the fork window.

#pragma once

#include <string>
#include <sys/types.h>
#include <vector>

namespace child_process {

enum class Direction {
    StdoutPipe,
    StdinPipe,
};

struct SpawnResult {
    pid_t pid = -1;
    int pipe_fd = -1;
};

[[nodiscard]] SpawnResult spawn(const std::string& program, const std::vector<std::string>& argv,
                                Direction direction);

// Wait up to `graceful_ms` for `pid` to exit; if still alive send SIGTERM
// and wait another `graceful_ms`; if STILL alive SIGKILL. Closes the pipe
// if `pipe_fd >= 0`. Safe to call with pid=-1 (no-op).
void reap(pid_t pid, int pipe_fd, int graceful_ms);

struct ShellSpawnResult {
    pid_t pid = -1;
    int stdout_fd = -1;
};

// fork-based: posix_spawn can't set PR_SET_PDEATHSIG or a fresh pgroup.
[[nodiscard]] ShellSpawnResult spawn_shell_group(const std::string& shell_cmd, int pdeathsig);

// reap() semantics, but signals the child's process group.
void reap_group(pid_t pid, int pipe_fd, int graceful_ms);

} // namespace child_process
