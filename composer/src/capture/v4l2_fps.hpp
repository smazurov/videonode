// v4l2_fps — pure helpers for reading the V4L2 capture frame rate.
//
// V4L2 reports the capture rate as `struct v4l2_fract timeperframe`, the
// time per frame in seconds = numerator / denominator; the frame rate is
// therefore denominator / numerator. These are split out as free functions
// so both the arithmetic and the VIDIOC_G_PARM call path are unit-testable
// without a real device: query_capture_fps takes an injectable ioctl
// invoker, so a test can supply a fake instead of touching a /dev/video fd.

#pragma once

#include <cstdint>
#include <functional>

namespace v4l2 {

// IoctlFn mirrors ::ioctl(fd, request, arg). Injected into query_capture_fps
// so tests can fake the kernel without a device; production passes a thin
// wrapper over the real (EINTR-retrying) ioctl.
using IoctlFn = std::function<int(int fd, unsigned long request, void* arg)>;

// fps_from_timeperframe converts a V4L2 timeperframe fraction
// (numerator/denominator seconds per frame) into a rounded fps. Returns 0
// when numerator is 0 (rate unknown / would divide by zero). NTSC-style
// fractional rates round to the nearest whole fps (e.g. 60000/1001 -> 60).
[[nodiscard]] uint32_t fps_from_timeperframe(uint32_t numerator, uint32_t denominator);

// query_capture_fps issues VIDIOC_G_PARM through `io` for the given fd and
// buffer type, then returns the rounded capture fps. Returns 0 on a
// negative fd, an ioctl failure, or an unknown rate.
[[nodiscard]] uint32_t query_capture_fps(const IoctlFn& io, int fd, uint32_t buf_type);

} // namespace v4l2
