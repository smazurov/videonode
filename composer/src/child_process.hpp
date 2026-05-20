// child_process — small helper around posix_spawn for child processes that
// communicate via a single inherited pipe.
//
// Both FfmpegPipeSource (reads NV12 from child's stdout) and FfmpegEncoder
// (writes NV12 to child's stdin) used to open-code the pipe2+posix_spawn
// dance with slightly different file-action setups. This consolidates them.
//
// Direction:
//   StdoutPipe: parent reads from `pipe_fd`; child's stdout goes to write end.
//   StdinPipe:  parent writes to `pipe_fd`; child's stdin comes from read end.
//
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
    StdoutPipe, // Parent reads from child's stdout via returned pipe_fd.
    StdinPipe,  // Parent writes to child's stdin via returned pipe_fd.
};

struct SpawnResult {
    pid_t pid = -1;
    int pipe_fd = -1;
};

// Spawn `program` with `argv` (argv[0] is conventionally program name).
// Sets up a single pipe per `direction`; returns the parent's end.
// Returns {-1, -1} on failure (stderr line printed with detail).
SpawnResult spawn(const std::string& program, const std::vector<std::string>& argv,
                  Direction direction);

// Wait up to `graceful_ms` for `pid` to exit; if still alive send SIGTERM
// and wait another `graceful_ms`; if STILL alive SIGKILL. Closes the pipe
// if `pipe_fd >= 0`. Safe to call with pid=-1 (no-op).
void reap(pid_t pid, int pipe_fd, int graceful_ms);

} // namespace child_process
