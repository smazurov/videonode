// v4l2_capture — minimal C++ V4L2 streaming layer for the videonode-source
// sidecar. Mirrors the public shape of pkg/linuxav/v4l2.Streamer (Go);
// covers the happy-path subset the sidecar needs:
//
//   open + QUERYCAP (detect multiplanar)
//   S_FMT (set pixel format, width, height)
//   REQBUFS + EXPBUF (allocate ring + get dma-buf fds)
//   QBUF / DQBUF (queue / dequeue ready buffers)
//   STREAMON / STREAMOFF
//   close (with fd cleanup)
//
// Single-plane (UVC) and multi-plane (rk_hdmirx) drivers both supported —
// detected from QUERYCAP and dispatched at the ioctl level. Multi-plane
// per-plane dma-buf fds are exposed as a vector in BufferRef.
//
// Source-change events (V4L2_EVENT_SOURCE_CHANGE), backoff, runloop, and
// thread-safe latest-frame snapshots live in V4l2Source (separate header,
// built on top of this). Capture loop semantics live there too — this
// class is one thread's worth of low-level access.

#pragma once

#include <cstddef>
#include <cstdint>
#include <string>
#include <utility>
#include <vector>

struct v4l2_event;

namespace v4l2 {

// StreamFormat is the capture configuration applied via VIDIOC_S_FMT.
// Multiplanar is derived from QUERYCAP, not chosen by the caller — Open
// detects and fills it on the Streamer.
struct StreamFormat {
    uint32_t pixel_format = 0; // V4L2_PIX_FMT_* fourcc
    uint32_t width = 0;
    uint32_t height = 0;
    // FPS as time-per-frame via VIDIOC_S_PARM. Some drivers ignore this
    // (rk_hdmirx is one). 0 = don't call S_PARM.
    uint32_t fps = 0;
};

// PlaneRef is one plane of a V4L2 buffer. For single-plane formats there's
// exactly one plane; for multi-plane there are two (or more). dma_buf_fd
// is owned by the Streamer; -1 until ExportBuffer is called.
struct PlaneRef {
    int dma_buf_fd = -1;
    uint32_t length = 0;
    uint32_t mmap_offset = 0;
};

// BufferRef is one of the kernel-allocated capture buffers in the ring.
// PrimaryDmaBuf is a convenience accessor for the most common case
// (single fd per buffer with chroma packed in the same dma-buf).
struct BufferRef {
    uint32_t index = 0;
    uint32_t length = 0;
    std::vector<PlaneRef> planes;

    int primary_dma_buf() const { return planes.empty() ? -1 : planes[0].dma_buf_fd; }
};

// DequeuedFrame is what DequeueBuffer returns: the index of a ready
// buffer plus kernel metadata. Consumers find the matching BufferRef by
// index and process its dma-buf, then re-queue with QueueBuffer.
struct DequeuedFrame {
    uint32_t index = 0;
    uint32_t bytesused = 0; // single-plane: payload; multi-plane: sum
    uint32_t sequence = 0;
    uint64_t timestamp_ns = 0; // monotonic, from kernel timestamp
    uint32_t flags = 0;
    std::vector<uint32_t> plane_bytesused; // multi-plane: per-plane
};

// Streamer wraps one capture device through the streaming lifecycle.
// Methods are NOT thread-safe individually; V4l2Source wraps a Streamer
// in its own thread and provides snapshot semantics for consumers.
class Streamer {
  public:
    // open() opens the device, QUERYCAPs it, fills multiplanar(). Returns
    // false on error with an internal error message logged to stderr.
    bool open(const std::string& device_path);

    // close() releases all dma-buf fds and the device fd. Idempotent.
    void close();

    ~Streamer();
    Streamer() = default;
    Streamer(const Streamer&) = delete;
    Streamer& operator=(const Streamer&) = delete;

    bool multiplanar() const { return multiplanar_; }
    int fd() const { return fd_; }
    const std::string& device_path() const { return device_path_; }

    // set_format() calls VIDIOC_S_FMT (and S_PARM if fps != 0). Returns
    // false on error.
    bool set_format(const StreamFormat& f);

    // get_format() calls VIDIOC_G_FMT and reads back the driver's current
    // settings. Useful for devices like rk_hdmirx that auto-negotiate the
    // capture format from the incoming HDMI source; the sidecar can take
    // "whatever HDMI is producing" without hardcoding.
    bool get_format(StreamFormat& out) const;

    // request_buffers() calls VIDIOC_REQBUFS for `count` mmap buffers and
    // queries each one via QUERYBUF to populate plane lengths/offsets.
    // Fills the returned vector; also caches it internally so close() can
    // free the fds. Subsequent calls re-request — the old set is cleaned.
    bool request_buffers(int count, std::vector<BufferRef>& out);

    // export_buffer() calls VIDIOC_EXPBUF for one (buffer index, plane).
    // Mutates the cached BufferRef so subsequent calls to buffers() see
    // the populated fd.
    bool export_buffer(uint32_t index, uint32_t plane, int& out_fd);

    // export_all_planes() loops over the cached buffer set, exporting
    // every plane. Convenience for the common case where the producer
    // wants every fd up front.
    bool export_all_planes();

    // queue_buffer() returns one buffer to the kernel ring (QBUF).
    bool queue_buffer(uint32_t index);

    // dequeue_buffer() waits up to timeout_ms for a ready frame, then
    // DQBUF. Returns false on timeout or error.
    bool dequeue_buffer(int timeout_ms, DequeuedFrame& out);

    bool stream_on();
    bool stream_off();

    // V4L2 source-change handling. Must subscribe BEFORE stream_on or
    // the kernel buffers events without notifying. drain_events()
    // returns true if any events were drained — caller should treat that
    // as "stream is stale, do a STREAMOFF/QBUF*/STREAMON cycle."
    bool subscribe_source_change();
    bool drain_events(bool* drained = nullptr);

    // subscribe_ctrl_event subscribes to V4L2_EVENT_CTRL for one control.
    // The kernel wakes poll(POLLPRI) when the control's value changes.
    // Returns false on error (e.g. EINVAL if the device doesn't expose that
    // control). Caller should typically tolerate failure here.
    bool subscribe_ctrl_event(uint32_t cid);

    // drain_events_typed reads all pending events and returns their raw
    // v4l2_event records so callers can inspect type + payload. Same
    // semantics as drain_events() — call when poll() reports POLLPRI.
    bool drain_events_typed(std::vector<struct v4l2_event>& out);

    // read_ctrl reads one control via VIDIOC_G_CTRL.
    bool read_ctrl(uint32_t cid, int32_t& out_value) const;

    // query_dv_timings_valid returns true if VIDIOC_QUERY_DV_TIMINGS
    // succeeds AND reports non-zero active dimensions. Used for HDMI
    // receivers as a "the source is producing a recognized signal" check.
    // Returns false on devices that don't implement DV timings (e.g. UVC).
    bool query_dv_timings_valid() const;

    // DV-timings probe states. Mirrors the Go pkg/linuxav/v4l2.SignalState
    // semantics; see pkg/linuxav/v4l2/signal.go for the proven mapping.
    enum class DvTimingsState {
        Locked,       // VIDIOC_QUERY_DV_TIMINGS ok + non-zero dimensions
        NoLink,       // ENOLINK — no cable detected
        Unstable,     // ENOLCK — cable in but signal unstable
        OutOfRange,   // ERANGE — signal outside supported timings
        NotSupported, // ENOTTY — device doesn't implement DV timings
        OtherError,   // any other errno from the ioctl
    };

    // query_dv_timings_state is the full-fidelity variant used when the
    // caller needs to distinguish "no cable" from "cable in, no lock".
    // The driver-truth for cable presence is QUERY_DV_TIMINGS, not the
    // DV_RX_POWER_PRESENT control (rk_hdmirx lies about that one).
    DvTimingsState query_dv_timings_state() const;

    // Full restart cycle after a source-change: STREAMOFF, re-queue all
    // cached buffers, STREAMON. The kernel's ring is reset; existing
    // dma-buf fds remain valid (we re-queue the same buffers).
    bool restart_streaming();

    // Buffer access (after request_buffers() succeeded). The vector is
    // valid until the next request_buffers() call or close().
    const std::vector<BufferRef>& buffers() const { return bufs_; }

    // mmap_buffer maps one previously-QUERYBUF'd buffer for CPU read. Used
    // by the MJPEG path to read variable-length JPEG bitstreams out of
    // V4L2 capture buffers (dma-buf fds aren't useful for that — we want
    // a normal pointer + bytesused). Single-plane only; UVC MJPEG is
    // always single-plane. The mapping persists until the next
    // request_buffers() call or close().
    bool mmap_buffer(uint32_t index, void*& out_ptr, size_t& out_size);

  private:
    uint32_t buf_type_() const; // CAPTURE vs CAPTURE_MPLANE
    bool query_buffer_(uint32_t index, BufferRef& out);
    void unmap_all_();

    int fd_ = -1;
    std::string device_path_;
    bool multiplanar_ = false;
    bool streaming_ = false;
    std::vector<BufferRef> bufs_;
    std::vector<std::pair<void*, size_t>> in_maps_;
};

} // namespace v4l2
