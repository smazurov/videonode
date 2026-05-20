// format_dispatch — translate a DRM fourcc string ("NV12" / "NV24" / "NV16"
// / "BG24") plus width+height into an egl_ctx::EglCtx::ImageDesc with the
// correct fourcc and per-plane pitches/offsets for semi-planar layouts
// when the producer left them zero.
//
// Lives in its own translation unit so the logic is unit-testable without
// pulling in EGL/GBM — tests/test_format_dispatch.cpp exercises every
// supported format. Driver-level support is a separate question; see
// tools/dmabuf-format-probe to enumerate what a given Mali userspace
// actually accepts.

#pragma once

#include "egl_ctx.hpp"

#include <cstdint>
#include <string_view>

namespace format_dispatch {

// fourcc_from_string converts a 4-char DRM fourcc string to its
// little-endian uint32 code. Returns 0 for inputs of any other length —
// callers usually treat 0 as "default to NV12".
uint32_t fourcc_from_string(std::string_view s);

// fill_image_desc populates ImageDesc.fourcc and derives missing per-plane
// pitches/offsets for known semi-planar formats. Single-plane layouts
// (BGR888) leave plane1_* zero. Producers that already populate the plane
// fields are passed through unchanged.
//
// fmt may be empty → defaults to NV12 (back-compat for senders that don't
// carry Format on the wire).
void fill_image_desc(egl_ctx::EglCtx::ImageDesc& d, std::string_view fmt, uint32_t width,
                     uint32_t height);

} // namespace format_dispatch
