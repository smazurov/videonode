#include "src/process/y4m_writer.hpp"

#include <cerrno>
#include <cstdio>
#include <cstring>
#include <unistd.h>

namespace vn::process {

namespace {

ssize_t default_write(int fd, const void* buf, size_t len) {
    return ::write(fd, buf, len);
}

} // namespace

Y4mWriter::Y4mWriter(int out_fd, int width, int height, int fps_num, int fps_den)
    : out_fd_(out_fd), width_(width), height_(height), fps_num_(fps_num), fps_den_(fps_den),
      write_fn_(&default_write) {}

bool Y4mWriter::WriteAll(std::span<const uint8_t> buf) {
    while (!buf.empty()) {
        ssize_t w = write_fn_(out_fd_, buf.data(), buf.size());
        if (w < 0) {
            if (errno == EINTR)
                continue;
            return false;
        }
        if (w == 0)
            return false;
        buf = buf.subspan(static_cast<size_t>(w));
    }
    return true;
}

bool Y4mWriter::WriteHeader() {
    char hdr[128];
    int n = std::snprintf(hdr, sizeof(hdr), "YUV4MPEG2 W%d H%d F%d:%d Ip A1:1 C420\n", width_,
                          height_, fps_num_, fps_den_);
    if (n <= 0 || static_cast<size_t>(n) >= sizeof(hdr))
        return false;
    return WriteAll(std::span(reinterpret_cast<const uint8_t*>(hdr), static_cast<size_t>(n)));
}

bool Y4mWriter::WriteFrameNV12(std::span<const uint8_t> y_plane, size_t y_stride,
                               std::span<const uint8_t> uv_plane, size_t uv_stride) {
    const size_t width = static_cast<size_t>(width_);
    const size_t height = static_cast<size_t>(height_);
    const size_t uv_rows = height / 2;
    const size_t u_size = (width / 2) * uv_rows;
    const size_t v_size = u_size;

    if (y_stride < width || uv_stride < width)
        return false;
    if (y_plane.size() < y_stride * height)
        return false;
    if (uv_plane.size() < uv_stride * uv_rows)
        return false;

    // Layout: [U... | V...] back to back so we can write each half as a
    // contiguous span without re-allocating per frame.
    if (chroma_scratch_.size() != u_size + v_size) {
        chroma_scratch_.assign(u_size + v_size, 0);
    }
    uint8_t* u_dst_base = chroma_scratch_.data();
    uint8_t* v_dst_base = chroma_scratch_.data() + u_size;
    const size_t uv_pairs_per_row = width / 2;
    for (size_t row = 0; row < uv_rows; ++row) {
        const uint8_t* src = uv_plane.data() + row * uv_stride;
        uint8_t* u_dst = u_dst_base + row * uv_pairs_per_row;
        uint8_t* v_dst = v_dst_base + row * uv_pairs_per_row;
        for (size_t i = 0, j = 0; i < width; i += 2, ++j) {
            u_dst[j] = src[i];
            v_dst[j] = src[i + 1];
        }
    }

    static constexpr uint8_t kFrameTag[] = {'F', 'R', 'A', 'M', 'E', '\n'};
    if (!WriteAll(std::span<const uint8_t>(kFrameTag)))
        return false;

    // Y: write row-by-row only when stride > width to drop padding; the
    // contiguous case is one syscall.
    if (y_stride == width) {
        if (!WriteAll(y_plane.subspan(0, width * height)))
            return false;
    } else {
        for (size_t row = 0; row < height; ++row) {
            if (!WriteAll(y_plane.subspan(row * y_stride, width)))
                return false;
        }
    }

    if (!WriteAll(std::span<const uint8_t>(u_dst_base, u_size)))
        return false;
    if (!WriteAll(std::span<const uint8_t>(v_dst_base, v_size)))
        return false;
    return true;
}

} // namespace vn::process
