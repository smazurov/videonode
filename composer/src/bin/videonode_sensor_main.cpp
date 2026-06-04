// videonode-sensor — AI perception sidecar. Dials an analysis composer's
// NV12 canvas SCM bus, hands the Y plane to an out-of-process detector child,
// and streams normalized Findings to the daemon over gRPC (Sensor service).
// Flags: --grpc-listen --sensor-id --upstream-scm --target-ref --model-id
//        --detector "<shell cmd>" --tick-ms.

#include "src/sensor/orchestrator.hpp"
#include "src/common/log_levels.hpp"
#include "src/common/raise_fd_limit.hpp"
#include "src/common/signal.hpp"
#include "version.hpp"

#include <atomic>
#include <cstdio>
#include <cstdlib>
#include <cstring>
#include <span>
#include <string>
#include <sys/prctl.h>
#include <unistd.h>

namespace {

std::atomic<bool> g_running{true};

const char* flag_value(std::span<char*> args, std::string_view name) {
    for (size_t i = 1; i + 1 < args.size(); ++i)
        if (name == args[i])
            return args[i + 1];
    return nullptr;
}

} // namespace

int main(int argc, char** argv) {
    vn::raise_fd_limit();
    ::setvbuf(stderr, nullptr, _IOLBF, 0);

    const std::span<char*> args(argv, static_cast<size_t>(argc));
    for (size_t i = 1; i < args.size(); ++i) {
        if (std::strcmp(args[i], "--version") == 0) {
            std::printf("videonode-sensor %s\n", vn::kVersion);
            return 0;
        }
    }

    sensor::Args a;
    if (const char* v = flag_value(args, "--grpc-listen"))
        a.grpc_listen = v;
    if (const char* v = flag_value(args, "--sensor-id"))
        a.sensor_id = v;
    if (const char* v = flag_value(args, "--model-id"))
        a.model_id = v;
    if (const char* v = flag_value(args, "--target-ref"))
        a.target_ref = v;
    if (const char* v = flag_value(args, "--upstream-scm"))
        a.scm_path = v;
    if (const char* v = flag_value(args, "--detector"))
        a.detector = v;
    if (const char* v = flag_value(args, "--tick-ms"))
        a.tick_ms = std::atoi(v);

    vn::signal::install_shutdown(g_running);
    ::prctl(PR_SET_PDEATHSIG, SIGTERM);
    if (::getppid() == 1)
        return 0;

    return sensor::Run(a, g_running);
}
