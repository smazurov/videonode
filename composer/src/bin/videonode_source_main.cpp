// videonode-source entry point — signal wiring + delegate to
// source::Run. Body lives in src/source/orchestrator.cpp.

#include "src/common/flags_compat.hpp"
#include "src/common/signal.hpp"
#include "src/source/orchestrator.hpp"
#include "version.hpp"

#include <absl/flags/parse.h>
#include <absl/flags/usage.h>

#include <atomic>
#include <cstdio>
#include <cstring>
#include <sys/prctl.h>
#include <unistd.h>

namespace {

std::atomic<bool> g_running{true};

} // namespace

int main(int argc, char** argv) {
    // stderr defaults to block-buffered when redirected to a file (which
    // the supervisor does: `>log 2>&1`). Force line-buffered so each log
    // line is visible to `tail -f` immediately instead of waiting for a
    // 4 KiB chunk to accumulate.
    ::setvbuf(stderr, nullptr, _IOLBF, 0);

    // absl::ParseCommandLine treats --version as an unknown flag (it only
    // owns --help / --helpfull / etc.). Intercept it before parsing so we
    // get the legacy `<binary> <version>` line that supervisors grep for.
    for (int i = 1; i < argc; ++i) {
        if (std::strcmp(argv[i], "--version") == 0) {
            std::printf("videonode-source %s\n", vn::kVersion);
            return 0;
        }
    }

    absl::SetProgramUsageMessage(
        "videonode-source — V4L2 capture → (RGA-CSC | JPEG-decode) → NV12 dma-buf → SCM_RIGHTS\n"
        "  with event-driven placeholder when the source is absent or in flux.");
    vn::flags::configure_help_filter();
    vn::flags::normalize_argv(argc, argv);
    absl::ParseCommandLine(argc, argv);
    source::Args a = source::BuildArgsFromFlags();
    vn::signal::install_shutdown(g_running);
    ::prctl(PR_SET_PDEATHSIG, SIGTERM);
    if (::getppid() == 1)
        return 0;
    return source::Run(a, g_running);
}
