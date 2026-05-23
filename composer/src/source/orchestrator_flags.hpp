// videonode-source CLI flag wiring. Split out of orchestrator.cpp so the
// orchestrator's main loop stays under the 500-line soft cap. The
// ABSL_FLAGs themselves are defined (with ODR) in orchestrator_flags.cpp
// — declaring them in a header would multiply-define the flag globals.
//
// The binary calls absl::ParseCommandLine, then BuildArgsFromFlags() to
// materialize a source::Args from the parsed flag values.
#pragma once

#include "src/source/args.hpp"

namespace source {

// BuildArgsFromFlags reads the ABSL_FLAGs defined in orchestrator_flags.cpp
// and returns a populated Args. Call after absl::ParseCommandLine.
[[nodiscard]] Args BuildArgsFromFlags();

} // namespace source
