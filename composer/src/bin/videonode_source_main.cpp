// videonode-source entry point — signal wiring + delegate to
// source::Run. Body lives in src/source/orchestrator.cpp.

#include "src/source/orchestrator.hpp"

#include <atomic>
#include <csignal>
#include <cstdio>
#include <sys/prctl.h>
#include <unistd.h>

namespace {

std::atomic<bool> g_running{true};

void on_signal(int) {
    g_running.store(false);
}

} // namespace

int main(int argc, char** argv) {
    // stderr defaults to block-buffered when redirected to a file (which
    // the supervisor does: `>log 2>&1`). Force line-buffered so each log
    // line is visible to `tail -f` immediately instead of waiting for a
    // 4 KiB chunk to accumulate.
    ::setvbuf(stderr, nullptr, _IOLBF, 0);

    source::Args a;
    if (!source::parse_args(argc, argv, a))
        return 2;
    std::signal(SIGINT, on_signal);
    std::signal(SIGTERM, on_signal);
    std::signal(SIGPIPE, SIG_IGN);
    ::prctl(PR_SET_PDEATHSIG, SIGTERM);
    if (::getppid() == 1)
        return 0;
    return source::Run(a, g_running);
}
