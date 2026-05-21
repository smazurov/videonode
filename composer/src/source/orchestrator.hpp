// SourceOrchestrator — argv parsing + capture/broadcast lifecycle for the
// videonode-source sidecar. Pulled out of bin/videonode_source_main.cpp so
// the binary entry point stays signal-handler-only. `Args` lives in
// args.hpp so broadcast.hpp can reference it without depending on the
// full orchestrator.
#pragma once

#include "src/source/args.hpp"

#include <atomic>

namespace source {

// Parse argv into Args. Returns false on unknown flag / missing value.
// `--help` / `--version` call exit() directly.
[[nodiscard]] bool parse_args(int argc, char** argv, Args& a);

// Print usage to stdout. `d` supplies default values for the message.
void print_help(const Args& d);

// Run the capture + broadcast loop until `running` goes false. Returns
// process exit code (0 = ok, non-zero = startup failure).
int Run(const Args& a, std::atomic<bool>& running);

} // namespace source
