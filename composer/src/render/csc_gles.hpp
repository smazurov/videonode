// csc_gles — GLES2 two-pass NV12 producer for the csc:: dispatcher.
//
// Validated end-to-end by composer/tools/csc-probe.cpp on Mesa
// (radeonsi, anv, panfrost should all work; nouveau's
// EGL_MESA_image_dma_buf_export support is patchier).
//
// Source format support:
//   - NV24 → NV12 ✅ (Phase 2 first cut)
//   - NV12 → NV12 = pass-through; caller skips us entirely.
//   - NV16 / YUYV / UYVY / BGR3 → NV12 = TODO follow-ups (returns false
//     with one-time log).
//
// Output contract (matches dmabuf_msg.hpp): NV12 single dma-buf, Y plane
// at offset 0 pitch dst_w, UV plane at offset dst_w*dst_h pitch dst_w.
// BT.601 limited range, MPEG-2 chroma siting — see the Phase 0 probe for
// the shader math.

#pragma once

#include "src/render/csc.hpp"

namespace csc_gles {

// One-time backend initialization. Returns false if the EGL/GLES stack
// can't be brought up (no /dev/dri/renderD128, missing extensions, etc.).
// Idempotent — re-calling after success is a no-op and returns true.
bool init();

// Run one src→dst conversion. Caller passes already-allocated dma-buf
// fds. Returns false on backend error or unsupported format.
bool convert(const csc::ConvertParams& src, const csc::ConvertParams& dst);

// Tear down. Idempotent. Not strictly required (process exit cleans up).
void shutdown();

} // namespace csc_gles
