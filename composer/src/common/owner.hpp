// gsl::owner<T> — annotation for clang-tidy's cppcoreguidelines-owning-memory.
// Zero-cost type alias that marks a raw pointer as owning its pointee.

#pragma once

namespace gsl {
template <typename T> using owner = T;
} // namespace gsl
