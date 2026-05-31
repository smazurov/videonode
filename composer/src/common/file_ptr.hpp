// file_ptr — RAII owner for C stdio FILE* handles used by the probe tools.
//
// `std::unique_ptr<FILE, decltype(&std::fclose)>` trips GCC's
// -Wignored-attributes: std::fclose carries function attributes that are
// stripped when its type is used as a template argument. A stateless deleter
// functor sidesteps that and keeps the pointer the size of a bare FILE*.

#pragma once

#include <cstdio>
#include <memory>

namespace vn {

struct FileCloser {
    void operator()(std::FILE* f) const noexcept {
        if (f != nullptr) {
            std::fclose(f);
        }
    }
};

using FilePtr = std::unique_ptr<std::FILE, FileCloser>;

} // namespace vn
