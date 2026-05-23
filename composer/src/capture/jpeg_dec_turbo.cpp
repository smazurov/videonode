#include "src/capture/jpeg_dec_turbo.hpp"

#include "src/common/log_levels.hpp"

#include <turbojpeg.h>

#include <cstring>

namespace jpeg_dec {

TurboJpegDec::~TurboJpegDec() {
    if (handle_) {
        tjDestroy(static_cast<tjhandle>(handle_));
        handle_ = nullptr;
    }
}

bool TurboJpegDec::init(int width, int height, std::vector<Slot> ring) {
    if (width <= 0 || height <= 0 || (width & 1) || (height & 1)) {
        vn::log::error("jpeg_dec_turbo: bad dims %dx%d (must be even, NV12)", width, height);
        return false;
    }
    if (ring.empty()) {
        vn::log::error("jpeg_dec_turbo: empty ring");
        return false;
    }
    for (const auto& s : ring) {
        if (s.fd < 0 || !s.mapped) {
            vn::log::error("jpeg_dec_turbo: ring slot fd=%d mapped=%p invalid", s.fd,
                           static_cast<void*>(s.mapped));
            return false;
        }
    }
    handle_ = tjInitDecompress();
    if (!handle_) {
        vn::log::error("jpeg_dec_turbo: tjInitDecompress failed: %s", tjGetErrorStr());
        return false;
    }
    width_ = width;
    height_ = height;
    ring_ = std::move(ring);
    next_ = 0;
    // Chroma scratch is sized lazily on the first decode() — its size
    // depends on the JPEG's subsampling, which we don't know yet.
    u_scratch_.clear();
    v_scratch_.clear();
    return true;
}

bool TurboJpegDec::decode(std::span<const uint8_t> jpeg, DecodedNv12& out) {
    if (!handle_ || ring_.empty())
        return false;
    tjhandle h = static_cast<tjhandle>(handle_);

    int jw = 0, jh = 0, jsubsamp = 0, jcs = 0;
    if (tjDecompressHeader3(h, jpeg.data(), static_cast<unsigned long>(jpeg.size()), &jw, &jh,
                            &jsubsamp, &jcs) != 0) {
        vn::log::error("jpeg_dec_turbo: tjDecompressHeader3: %s", tjGetErrorStr2(h));
        return false;
    }
    if (jw != width_ || jh != height_) {
        vn::log::error("jpeg_dec_turbo: dim mismatch jpeg=%dx%d expected=%dx%d", jw, jh, width_,
                       height_);
        return false;
    }
    if (jsubsamp != TJSAMP_420 && jsubsamp != TJSAMP_422) {
        vn::log::error("jpeg_dec_turbo: unsupported subsampling=%d (only 4:2:0/TJSAMP_420 and "
                       "4:2:2/TJSAMP_422)",
                       jsubsamp);
        return false;
    }

    // Native chroma-plane geometry from TurboJPEG. For 4:2:0 it's
    // (W/2)x(H/2); for 4:2:2 it's (W/2)xH (full vertical, half horizontal).
    const int chroma_pw = tjPlaneWidth(1, width_, jsubsamp);
    const int chroma_ph = tjPlaneHeight(1, height_, jsubsamp);
    const std::size_t plane_bytes =
        static_cast<std::size_t>(chroma_pw) * static_cast<std::size_t>(chroma_ph);
    if (u_scratch_.size() < plane_bytes) {
        u_scratch_.assign(plane_bytes, 0);
        v_scratch_.assign(plane_bytes, 0);
    }

    Slot& s = ring_[next_];
    next_ = (next_ + 1) % ring_.size();

    unsigned char* planes[3] = {s.mapped, u_scratch_.data(), v_scratch_.data()};
    int strides[3] = {width_, chroma_pw, chroma_pw};
    if (tjDecompressToYUVPlanes(h, jpeg.data(), static_cast<unsigned long>(jpeg.size()), planes,
                                width_, strides, height_, 0) != 0) {
        vn::log::error("jpeg_dec_turbo: tjDecompressToYUVPlanes: %s", tjGetErrorStr2(h));
        return false;
    }

    // Convert native chroma → NV12's 4:2:0 interleaved UV. NV12 chroma
    // plane geometry is (W/2)x(H/2). For 4:2:0 input it's a straight
    // interleave; for 4:2:2 we average pairs of chroma rows vertically.
    const int nv12_cw = width_ / 2;
    const int nv12_ch = height_ / 2;
    uint8_t* uv = s.mapped + std::size_t(width_) * height_;
    const uint8_t* up = u_scratch_.data();
    const uint8_t* vp = v_scratch_.data();

    if (jsubsamp == TJSAMP_420) {
        const std::size_t n = static_cast<std::size_t>(nv12_cw) * nv12_ch;
        for (std::size_t i = 0; i < n; ++i) {
            uv[2 * i] = up[i];
            uv[2 * i + 1] = vp[i];
        }
    } else { // TJSAMP_422: vertically downsample 2:1
        for (int y = 0; y < nv12_ch; ++y) {
            const uint8_t* u0 = up + (2 * y) * chroma_pw;
            const uint8_t* u1 = up + (2 * y + 1) * chroma_pw;
            const uint8_t* v0 = vp + (2 * y) * chroma_pw;
            const uint8_t* v1 = vp + (2 * y + 1) * chroma_pw;
            uint8_t* row = uv + std::size_t(y) * 2 * nv12_cw;
            for (int x = 0; x < nv12_cw; ++x) {
                row[2 * x] = static_cast<uint8_t>((u0[x] + u1[x] + 1) >> 1);
                row[2 * x + 1] = static_cast<uint8_t>((v0[x] + v1[x] + 1) >> 1);
            }
        }
    }

    out.fd = s.fd;
    out.width = width_;
    out.height = height_;
    out.y_pitch = static_cast<uint32_t>(width_);
    out.uv_pitch = static_cast<uint32_t>(width_);
    out.y_offset = 0;
    out.uv_offset = static_cast<uint32_t>(width_) * static_cast<uint32_t>(height_);
    return true;
}

} // namespace jpeg_dec
