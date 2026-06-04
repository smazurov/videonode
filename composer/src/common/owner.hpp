// gsl::owner<T> — annotation for clang-tidy's cppcoreguidelines-owning-memory.

#pragma once

namespace gsl {
template <typename T> using owner = T;
}
