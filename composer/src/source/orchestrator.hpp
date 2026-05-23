// SourceOrchestrator — argv parsing + capture/broadcast lifecycle for the
// videonode-source sidecar. Pulled out of bin/videonode_source_main.cpp so
// the binary entry point stays signal-handler-only. `Args` lives in
// args.hpp so broadcast.hpp can reference it without depending on the
// full orchestrator.
//
// CLI flags are declared with ABSL_FLAG inside orchestrator.cpp. The
// binary calls absl::ParseCommandLine, then BuildArgsFromFlags() to
// materialize an Args.
#pragma once

#include "src/source/args.hpp"

#include <atomic>

namespace source {

// BuildArgsFromFlags reads the ABSL_FLAGs declared in orchestrator.cpp
// and returns a populated Args. Call after absl::ParseCommandLine.
[[nodiscard]] Args BuildArgsFromFlags();

// Run the capture + broadcast loop until `running` goes false. Returns
// process exit code (0 = ok, non-zero = startup failure).
int Run(const Args& a, std::atomic<bool>& running);

} // namespace source
