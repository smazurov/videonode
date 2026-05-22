// csc_gles — GLES2 two-pass NV12 producer for the csc:: dispatcher.
//
// Validated end-to-end by composer/tools/csc-probe.cpp on Mesa
// (radeonsi, anv, panfrost should all work; nouveau's
// EGL_MESA_image_dma_buf_export support is patchier).
//
// Source format support:
//   - NV24 → NV12 ✅ (Phase 2 first cut)
//   - NV12 → NV12 ✅ (shader-driven copy — orchestrator unconditionally
//     dispatches through csc::convert for DecodeMode::Rga, so we cannot
//     rely on the caller skipping us).
//   - NV16 / YUYV / UYVY / BGR3 → NV12 = TODO follow-ups (returns false
//     with one-time log). See GitHub issue #6.
//
// Output contract (matches dmabuf_msg.hpp): NV12 single dma-buf, Y plane
// at offset 0 pitch dst_w, UV plane at offset dst_w*dst_h pitch dst_w.
// BT.601 limited range, MPEG-2 chroma siting — see the Phase 0 probe for
// the shader math.

#pragma once

#include "src/render/csc.hpp"

struct gbm_device;

namespace csc_gles {

// One-time backend initialization. Returns false if the EGL/GLES stack
// can't be brought up (no /dev/dri/renderD128, missing extensions, etc.).
// Idempotent — re-calling after success is a no-op and returns true.
[[nodiscard]] bool init();

// Run one src→dst conversion. Caller passes already-allocated dma-buf
// fds. Returns false on backend error or unsupported format.
[[nodiscard]] bool convert(const csc::ConvertParams& src, const csc::ConvertParams& dst);

// Tear down. Idempotent. Not strictly required (process exit cleans up).
void shutdown();

// Access csc_gles's internal gbm_device. Only valid after a successful
// init(). Callers that allocate source/destination dma-bufs may want to
// share this device to avoid Mesa cross-gbm_device renderbuffer-import
// pitfalls on radeonsi. Returns nullptr if init() never ran.
[[nodiscard]] gbm_device* gbm_device_for_io();

} // namespace csc_gles
