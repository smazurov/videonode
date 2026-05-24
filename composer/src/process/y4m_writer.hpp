// y4m_writer — YUV4MPEG2 streaming helper for videonode-sink.
//
// Owns an output file descriptor (caller-supplied, e.g. STDOUT_FILENO) plus
// stream dimensions, framerate, and a reusable chroma scratch buffer used to
// deinterleave NV12 UV → I420 U + V on the way out. Wraps the raw `::write`
// calls in an EINTR/partial-write loop so the streaming loop in
// videonode-sink doesn't have to.
//
// Wire format (per YUV4MPEG2 spec):
//   header  : `YUV4MPEG2 W%d H%d F%d:%d Ip A1:1 C420\n`
//   frame   : `FRAME\n` <Y rows tightly packed> <U rows tightly packed>
//                                                <V rows tightly packed>
//
// NV12 UV layout on input is `U0V0U1V1…` per row at half-vertical-res; the
// writer splits into separate U and V output planes.
//
// `WriteHeader` must be called exactly once before the first frame.

#pragma once

#include <cstddef>
#include <cstdint>
#include <span>
#include <sys/types.h>
#include <vector>

namespace vn::process {

class Y4mWriter {
  public:
    // Construct a writer bound to `out_fd` (not owned; caller closes).
    // Stream metadata is fixed at construction — recreate the writer if
    // dimensions or framerate change mid-stream.
    Y4mWriter(int out_fd, int width, int height, int fps_num, int fps_den);

    // Emit the YUV4MPEG2 stream header. Call once, before any frames.
    [[nodiscard]] bool WriteHeader();

    // Emit one FRAME from an NV12 source. Y is copied row-by-row (honoring
    // `y_stride` ≥ width); UV is deinterleaved into U and V output planes
    // honoring `uv_stride` ≥ width (UV row stride; UV row count = height/2).
    // Returns false if any write fails (e.g. pipe closed) — the caller
    // should treat that as terminal.
    [[nodiscard]] bool WriteFrameNV12(std::span<const uint8_t> y_plane, size_t y_stride,
                                      std::span<const uint8_t> uv_plane, size_t uv_stride);

    // Test seam: override the syscall used to write bytes. The default
    // implementation calls ::write(out_fd_, ...). Returning a value < 0
    // simulates an error; returning 0 simulates EOF; returning a value
    // strictly less than `len` simulates a short write (which the writer
    // must loop around).
    using WriteFn = ssize_t (*)(int fd, const void* buf, size_t len);
    void SetWriteFnForTest(WriteFn fn) { write_fn_ = fn; }

  private:
    [[nodiscard]] bool WriteAll(std::span<const uint8_t> buf);

    int out_fd_;
    int width_;
    int height_;
    int fps_num_;
    int fps_den_;
    std::vector<uint8_t> chroma_scratch_; // holds deinterleaved U then V
    WriteFn write_fn_ = nullptr;
};

} // namespace vn::process
