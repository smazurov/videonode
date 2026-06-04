// Lives in its own translation unit so the logic is unit-testable without
// pulling in EGL/GBM — tests/test_format_dispatch.cpp exercises every
// supported format. Driver-level support is a separate question; see
// tools/dmabuf-format-probe to enumerate what a given Mali userspace
// actually accepts.

#pragma once

#include "src/render/egl_ctx.hpp"

#include <cstdint>
#include <string_view>

namespace format_dispatch {

// fourcc_from_string converts a 4-char DRM fourcc string to its
// little-endian uint32 code. Returns 0 for inputs of any other length —
// callers usually treat 0 as "default to NV12".
uint32_t fourcc_from_string(std::string_view s);

// fmt may be empty → defaults to NV12 (back-compat for senders that don't
// carry Format on the wire).
void fill_image_desc(egl_ctx::EglCtx::ImageDesc& d, std::string_view fmt, uint32_t width,
                     uint32_t height);

} // namespace format_dispatch
