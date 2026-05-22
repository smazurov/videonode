#include "src/process/child_process.hpp"

#include <cerrno>
#include <csignal>
#include <cstdio>
#include <cstring>
#include <fcntl.h>
#include <spawn.h>
#include <sys/wait.h>
#include <unistd.h>

extern char** environ;

namespace child_process {

SpawnResult spawn(const std::string& program, const std::vector<std::string>& argv,
                  Direction direction) {
    int pipefd[2];
    if (::pipe2(pipefd, O_CLOEXEC) < 0) {
        fprintf(stderr, "child_process: pipe2: %s\n", strerror(errno));
        return {};
    }
    int read_end = pipefd[0];
    int write_end = pipefd[1];

    int parent_end = -1;
    int child_dup_target = -1;
    int child_fd = -1;
    int parent_close = -1;

    switch (direction) {
    case Direction::StdoutPipe:
        parent_end = read_end;
        child_dup_target = STDOUT_FILENO;
        child_fd = write_end;
        parent_close = write_end;
        break;
    case Direction::StdinPipe:
        parent_end = write_end;
        child_dup_target = STDIN_FILENO;
        child_fd = read_end;
        parent_close = read_end;
        break;
    }

    posix_spawn_file_actions_t fa;
    posix_spawn_file_actions_init(&fa);
    posix_spawn_file_actions_addclose(&fa, parent_end);
    posix_spawn_file_actions_adddup2(&fa, child_fd, child_dup_target);
    posix_spawn_file_actions_addclose(&fa, child_fd);

    std::vector<char*> argv_c;
    argv_c.reserve(argv.size() + 1);
    for (const auto& s : argv)
        argv_c.push_back(const_cast<char*>(s.c_str()));
    argv_c.push_back(nullptr);

    pid_t pid = -1;
    int rc = posix_spawnp(&pid, program.c_str(), &fa, nullptr, argv_c.data(), environ);
    posix_spawn_file_actions_destroy(&fa);
    ::close(parent_close);

    if (rc != 0) {
        ::close(parent_end);
        fprintf(stderr, "child_process: posix_spawnp(%s): %s\n", program.c_str(), strerror(rc));
        return {};
    }
    return {.pid = pid, .pipe_fd = parent_end};
}

void reap(pid_t pid, int pipe_fd, int graceful_ms) {
    if (pipe_fd >= 0)
        ::close(pipe_fd);
    if (pid <= 0)
        return;

    auto wait_for = [&](int total_ms) -> bool {
        int status = 0;
        for (int waited = 0; waited <= total_ms; waited += 100) {
            if (::waitpid(pid, &status, WNOHANG) == pid)
                return true;
            ::usleep(100 * 1000);
        }
        return false;
    };

    if (wait_for(graceful_ms))
        return;
    ::kill(pid, SIGTERM);
    if (wait_for(graceful_ms))
        return;
    ::kill(pid, SIGKILL);
    int status = 0;
    ::waitpid(pid, &status, 0);
}

} // namespace child_process
