#pragma once

#include <cstddef>
#include <cstdint>
#include <optional>
#include <span>
#include <string>
#include <utility>
#include <vector>

struct v4l2_event;

namespace v4l2 {

// The pipeline always emits limited range, so only the matrix is tracked.
enum class ColorMatrix { Bt601, Bt709 };

[[nodiscard]] ColorMatrix resolve_matrix(uint32_t colorspace, uint32_t ycbcr_enc, uint32_t height);

struct StreamFormat {
    uint32_t pixel_format = 0; // V4L2_PIX_FMT_* fourcc
    uint32_t width = 0;
    uint32_t height = 0;
    // FPS as time-per-frame via VIDIOC_S_PARM. Some drivers ignore this
    // (rk_hdmirx is one). 0 = don't call S_PARM.
    uint32_t fps = 0;
    ColorMatrix color_matrix = ColorMatrix::Bt601;
};

// dma_buf_fd is owned by the Streamer; -1 until ExportBuffer is called.
struct PlaneRef {
    int dma_buf_fd = -1;
    uint32_t length = 0;
    uint32_t mmap_offset = 0;
};

struct BufferRef {
    uint32_t index = 0;
    uint32_t length = 0;
    std::vector<PlaneRef> planes;

    [[nodiscard]] int primary_dma_buf() const { return planes.empty() ? -1 : planes[0].dma_buf_fd; }
};

struct DequeuedFrame {
    uint32_t index = 0;
    uint32_t bytesused = 0; // single-plane: payload; multi-plane: sum
    uint32_t sequence = 0;
    uint64_t timestamp_ns = 0; // monotonic, from kernel timestamp
    uint32_t flags = 0;
    std::vector<uint32_t> plane_bytesused; // multi-plane: per-plane
};

// Methods are NOT thread-safe individually; V4l2Source wraps a Streamer
// in its own thread and provides snapshot semantics for consumers.
class Streamer {
  public:
    // Pass quiet=true on retry paths (1 Hz reopen loop) to suppress the
    // per-attempt open() error log and preserve errno for the caller to classify.
    [[nodiscard]] bool open(const std::string& device_path, bool quiet = false);

    // Idempotent.
    void close();

    ~Streamer();
    Streamer() = default;
    Streamer(const Streamer&) = delete;
    Streamer& operator=(const Streamer&) = delete;

    [[nodiscard]] bool multiplanar() const { return multiplanar_; }
    [[nodiscard]] int fd() const { return fd_; }
    [[nodiscard]] const std::string& device_path() const { return device_path_; }

    [[nodiscard]] bool set_format(const StreamFormat& f);

    // Useful for devices like rk_hdmirx that auto-negotiate the capture format
    // from the incoming HDMI source; the sidecar can take "whatever HDMI is
    // producing" without hardcoding.
    [[nodiscard]] bool get_format(StreamFormat& out) const;

    // Subsequent calls re-request — the old set is cleaned.
    [[nodiscard]] bool request_buffers(int count, std::vector<BufferRef>& out);

    // Mutates the cached BufferRef so subsequent calls to buffers() see
    // the populated fd.
    [[nodiscard]] bool export_buffer(uint32_t index, uint32_t plane, int& out_fd);

    [[nodiscard]] bool export_all_planes();

    [[nodiscard]] bool queue_buffer(uint32_t index);

    [[nodiscard]] bool dequeue_buffer(int timeout_ms, DequeuedFrame& out);

    [[nodiscard]] bool stream_on();
    [[nodiscard]] bool stream_off();

    // Must subscribe BEFORE stream_on or the kernel buffers events without
    // notifying. drain_events() returns true if any events were drained —
    // caller should treat that as "stream is stale, do a STREAMOFF/QBUF*/STREAMON cycle."
    [[nodiscard]] bool subscribe_source_change();
    [[nodiscard]] bool drain_events(bool* drained = nullptr);

    // The kernel wakes poll(POLLPRI) when the control's value changes.
    // Returns false on error (e.g. EINVAL if the device doesn't expose that
    // control). Caller should typically tolerate failure here.
    [[nodiscard]] bool subscribe_ctrl_event(uint32_t cid);

    // Same semantics as drain_events() — call when poll() reports POLLPRI.
    [[nodiscard]] bool drain_events_typed(std::vector<struct v4l2_event>& out);

    [[nodiscard]] bool read_ctrl(uint32_t cid, int32_t& out_value) const;

    // Returns false on devices that don't implement DV timings (e.g. UVC).
    [[nodiscard]] bool query_dv_timings_valid() const;

    // Mirrors the Go pkg/linuxav/v4l2.SignalState semantics;
    // see pkg/linuxav/v4l2/signal.go for the proven mapping.
    enum class DvTimingsState {
        Locked,       // VIDIOC_QUERY_DV_TIMINGS ok + non-zero dimensions
        NoLink,       // ENOLINK — no cable detected
        Unstable,     // ENOLCK — cable in but signal unstable
        OutOfRange,   // ERANGE — signal outside supported timings
        NotSupported, // ENOTTY — device doesn't implement DV timings
        OtherError,   // any other errno from the ioctl
    };

    // The driver-truth for cable presence is QUERY_DV_TIMINGS, not the
    // DV_RX_POWER_PRESENT control (rk_hdmirx lies about that one).
    [[nodiscard]] DvTimingsState query_dv_timings_state() const;

    // The kernel's ring is reset; existing dma-buf fds remain valid
    // (we re-queue the same buffers).
    [[nodiscard]] bool restart_streaming();

    // Valid until the next request_buffers() call or close().
    [[nodiscard]] const std::vector<BufferRef>& buffers() const { return bufs_; }

    // Used by the MJPEG path to read variable-length JPEG bitstreams out of
    // V4L2 capture buffers (dma-buf fds aren't useful for that — we want
    // a normal pointer + bytesused). Single-plane only; UVC MJPEG is
    // always single-plane. The returned span is valid until the next
    // request_buffers() call or close(). Returns std::nullopt on failure
    // (errno set).
    std::optional<std::span<std::byte>> mmap_buffer_span(uint32_t index);

  private:
    [[nodiscard]] uint32_t buf_type_() const; // CAPTURE vs CAPTURE_MPLANE
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
