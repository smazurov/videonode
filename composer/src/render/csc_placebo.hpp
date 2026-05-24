// csc_placebo — libplacebo NV12 producer for the csc:: dispatcher.
//
// Replaces csc_gles. Uses pl_renderer (OpenGL backend) to convert
// NV24 → NV12 and NV12 → NV12 (passthrough). The OpenGL backend reuses
// the existing EGL/GBM infrastructure; Vulkan is a future option gated
// on HAVE_VULKAN at build time.

#pragma once

#include "src/render/csc.hpp"

struct gbm_device;

namespace csc_placebo {

[[nodiscard]] bool init();
[[nodiscard]] bool convert(const csc::ConvertParams& src, const csc::ConvertParams& dst);
void shutdown();
[[nodiscard]] gbm_device* gbm_device_for_io();

} // namespace csc_placebo
