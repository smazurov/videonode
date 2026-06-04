// Vendored from Android's libbase (system/libbase/include/android-base/unique_fd.h),
// trimmed to the subset videonode-composer needs (no Fdsan tagging, no Android
// log integration). Upstream is Apache-2.0; the original copyright header is
// reproduced verbatim below.
//
// The Android type lives in `android::base::unique_fd`. We re-export it as
// `vn::base::unique_fd` to keep call sites uncluttered and to make the
// dependency direction "we vendor it" rather than "we depend on Android base".
//
// ─────────────────────────────────────────────────────────────────────────
// Copyright (C) 2016 The Android Open Source Project
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
// ─────────────────────────────────────────────────────────────────────────

#pragma once

#include <unistd.h>

#include <utility>

namespace android::base {

// The wrapper is intentionally minimal: it does NOT track caller identity
// (no fdsan), does NOT call any logging on close, and does NOT support
// `dup()`-like helpers. Anything more elaborate belongs at the call site.
struct DefaultCloser {
    static void Close(int fd) noexcept {
        // ::close can fail (EINTR, EIO) but there is no useful recovery
        // — the fd is gone from the process either way per POSIX.
        if (fd >= 0) {
            ::close(fd);
        }
    }
};

template <typename Closer> class unique_fd_impl {
  public:
    unique_fd_impl() noexcept = default;

    explicit unique_fd_impl(int fd) noexcept : fd_(fd) {}

    ~unique_fd_impl() { reset(); }

    unique_fd_impl(const unique_fd_impl&) = delete;
    unique_fd_impl& operator=(const unique_fd_impl&) = delete;

    unique_fd_impl(unique_fd_impl&& other) noexcept : fd_(other.release()) {}

    unique_fd_impl& operator=(unique_fd_impl&& other) noexcept {
        reset(other.release());
        return *this;
    }

    void reset(int new_value = -1) noexcept {
        if (fd_ != -1) {
            Closer::Close(fd_);
        }
        fd_ = new_value;
    }

    [[nodiscard]] int get() const noexcept { return fd_; }

    // Implicit conversion to int is convenient at syscall boundaries
    // (e.g., `::read(fd, ...)`) but [[nodiscard]] keeps us from
    // accidentally dropping a freshly-constructed fd.
    [[nodiscard]] operator int() const noexcept { return fd_; }

    [[nodiscard]] int release() noexcept {
        int ret = fd_;
        fd_ = -1;
        return ret;
    }

    explicit operator bool() const noexcept { return fd_ >= 0; }

    [[nodiscard]] bool ok() const noexcept { return fd_ >= 0; }

  private:
    int fd_ = -1;
};

using unique_fd = unique_fd_impl<DefaultCloser>;

} // namespace android::base

namespace vn::base {

// Project-local alias. Keeps call sites short and lets us swap the
// underlying implementation later without churning every include.
using unique_fd = android::base::unique_fd;

} // namespace vn::base
