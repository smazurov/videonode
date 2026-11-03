#include "src/sink/nv12_pack.hpp"

#include "src/common/log_levels.hpp"

#include <cerrno>
#include <cstring>
#include <linux/dma-buf.h>
#include <sys/ioctl.h>
#include <sys/mman.h>
#include <unistd.h>

#ifdef __SSE4_1__
#include <immintrin.h>
#endif

namespace sink {

namespace {

void dmabuf_sync(int fd, uint64_t phase) {
    struct dma_buf_sync sync{};
    sync.flags = phase | DMA_BUF_SYNC_READ;
    ::ioctl(fd, DMA_BUF_IOCTL_SYNC, &sync);
}

// movntdqa bypasses the write-combining serialization that makes plain loads from GPU dma-bufs
// ~100x slower.
void streaming_memcpy(uint8_t* dst, const uint8_t* src, size_t len) {
#ifdef __SSE4_1__
    size_t n = len / 16;
    std::span<uint8_t> dst_bytes(dst, len);
    std::span<const uint8_t> src_bytes(src, len);
    auto src_block = [&](size_t k) {
        return reinterpret_cast<__m128i*>(
            const_cast<uint8_t*>(src_bytes.subspan(k * 16, 16).data()));
    };
    auto dst_block = [&](size_t k) {
        return reinterpret_cast<__m128i*>(dst_bytes.subspan(k * 16, 16).data());
    };
    size_t i = 0;
    for (; i + 4 <= n; i += 4) {
        __m128i r0 = _mm_stream_load_si128(src_block(i + 0));
        __m128i r1 = _mm_stream_load_si128(src_block(i + 1));
        __m128i r2 = _mm_stream_load_si128(src_block(i + 2));
        __m128i r3 = _mm_stream_load_si128(src_block(i + 3));
        _mm_store_si128(dst_block(i + 0), r0);
        _mm_store_si128(dst_block(i + 1), r1);
        _mm_store_si128(dst_block(i + 2), r2);
        _mm_store_si128(dst_block(i + 3), r3);
    }
    for (; i < n; ++i)
        _mm_store_si128(dst_block(i), _mm_stream_load_si128(src_block(i)));
    size_t tail = len & 15;
    if (tail > 0)
        std::memcpy(dst_bytes.subspan(n * 16).data(), src_bytes.subspan(n * 16).data(), tail);
    _mm_sfence();
#else
    std::memcpy(dst, src, len);
#endif
}

void copy_plane(std::span<uint8_t> dst, const uint8_t* src, size_t width, size_t pitch,
                size_t rows) {
    if (pitch == width) {
        streaming_memcpy(dst.data(), src, width * rows);
        return;
    }
    std::span<const uint8_t> src_span(src, pitch * rows);
    for (size_t r = 0; r < rows; ++r)
        streaming_memcpy(dst.subspan(r * width, width).data(), src_span.subspan(r * pitch).data(),
                         width);
}

struct Mapping {
    void* addr = MAP_FAILED;
    size_t size = 0;
    int fd = -1;

    Mapping(int map_fd, size_t map_size) : size(map_size), fd(map_fd) {
        addr = ::mmap(nullptr, size, PROT_READ, MAP_SHARED, fd, 0);
        if (addr != MAP_FAILED)
            dmabuf_sync(fd, DMA_BUF_SYNC_START);
    }
    ~Mapping() {
        if (addr == MAP_FAILED)
            return;
        dmabuf_sync(fd, DMA_BUF_SYNC_END);
        ::munmap(addr, size);
    }
    Mapping(const Mapping&) = delete;
    Mapping& operator=(const Mapping&) = delete;
    Mapping(Mapping&&) = delete;
    Mapping& operator=(Mapping&&) = delete;

    [[nodiscard]] bool ok() const { return addr != MAP_FAILED; }
    [[nodiscard]] std::span<const uint8_t> bytes() const {
        return {static_cast<const uint8_t*>(addr), size};
    }
};

} // namespace

std::span<uint8_t> AlignedScratch::get(size_t bytes) {
    if (buf_.size() < bytes + 15)
        buf_.resize(bytes + 15);
    const auto base = reinterpret_cast<uintptr_t>(buf_.data());
    const size_t pad = ((base + 15) & ~uintptr_t(15)) - base;
    return std::span<uint8_t>(buf_).subspan(pad, bytes);
}

std::span<const uint8_t> pack_nv12(const scm_rights_source::FrameView& v, AlignedScratch& scratch) {
    if (v.fd < 0 || v.width <= 0 || v.height <= 0)
        return {};
    const auto width = static_cast<size_t>(v.width);
    const auto height = static_cast<size_t>(v.height);
    const size_t y_pitch = v.plane0_pitch != 0 ? v.plane0_pitch : width;
    const size_t uv_pitch = v.plane1_pitch != 0 ? v.plane1_pitch : width;
    if (y_pitch < width || uv_pitch < width) {
        vn::log::error("videonode-sink: NV12 stride < width (y=%zu uv=%zu w=%zu); dropping frame",
                       y_pitch, uv_pitch, width);
        return {};
    }
    const size_t uv_rows = height / 2;
    const size_t uv_raw_bytes = uv_pitch * uv_rows;
    std::span<uint8_t> out = scratch.get(width * height + width * uv_rows);

    const size_t y_map_size = v.plane0_offset + y_pitch * height +
                              (v.plane1_fd >= 0 ? 0 : uv_raw_bytes + v.plane1_offset);
    Mapping y_map(v.fd, y_map_size);
    if (!y_map.ok()) {
        vn::log::error("videonode-sink: mmap y_fd=%d failed: %s", v.fd, std::strerror(errno));
        return {};
    }
    const uint8_t* y_base = y_map.bytes().subspan(v.plane0_offset).data();
    copy_plane(out.subspan(0, width * height), y_base, width, y_pitch, height);

    if (v.plane1_fd >= 0) {
        Mapping uv_map(v.plane1_fd, v.plane1_offset + uv_raw_bytes);
        if (!uv_map.ok()) {
            vn::log::error("videonode-sink: mmap uv_fd=%d failed: %s", v.plane1_fd,
                           std::strerror(errno));
            return {};
        }
        copy_plane(out.subspan(width * height), uv_map.bytes().subspan(v.plane1_offset).data(),
                   width, uv_pitch, uv_rows);
    } else {
        const size_t uv_off =
            v.plane1_offset != 0 ? v.plane1_offset : v.plane0_offset + y_pitch * height;
        copy_plane(out.subspan(width * height), y_map.bytes().subspan(uv_off).data(), width,
                   uv_pitch, uv_rows);
    }
    return out;
}

bool write_full(int fd, std::span<const uint8_t> buf) {
    while (!buf.empty()) {
        const ssize_t w = ::write(fd, buf.data(), buf.size());
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

} // namespace sink
