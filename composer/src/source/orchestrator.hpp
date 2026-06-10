#pragma once

#include "src/source/args.hpp"

#include <atomic>

namespace source {

int Run(const Args& a, std::atomic<bool>& running);

// Pipe-mode loop (--pipe-cmd): spawned child's y4m stdout instead of V4L2.
int RunPipe(const Args& a, std::atomic<bool>& running);

} // namespace source
