#pragma once

#include "src/source/args.hpp"

#include <atomic>

namespace source {

int Run(const Args& a, std::atomic<bool>& running);

}
