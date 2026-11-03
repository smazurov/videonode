#pragma once

#include <cstdint>
#include <functional>
#include <span>
#include <vector>

namespace nut {

struct VideoParams {
    int width = 0;
    int height = 0;
    int fps = 0;
};

struct AudioParams {
    int sample_rate = 0;
    int channels = 0;
};

using ByteSink = std::function<bool(std::span<const uint8_t>)>;

[[nodiscard]] uint32_t crc32(uint32_t crc, std::span<const uint8_t> bytes);
void put_v(std::vector<uint8_t>& out, uint64_t v);
void put_s(std::vector<uint8_t>& out, int64_t v);

// NUT v4 in pipe mode: a single syncpoint, so ffmpeg's demuxer keeps no per-packet state.
class Writer {
  public:
    explicit Writer(ByteSink sink) : sink_(std::move(sink)) {}

    [[nodiscard]] bool write_header(const VideoParams& video, std::span<const AudioParams> audio);
    [[nodiscard]] bool write_frame(uint32_t stream, int64_t pts, std::span<const uint8_t> data);

    [[nodiscard]] static uint32_t video_stream() { return 0; }
    [[nodiscard]] static uint32_t audio_stream(uint32_t k) { return 1 + k; }

  private:
    struct StreamInfo {
        uint32_t time_base_id = 0;
    };

    [[nodiscard]] bool put_packet(uint64_t startcode, std::span<const uint8_t> payload);
    [[nodiscard]] bool write_main_header();
    [[nodiscard]] bool write_stream_headers(const VideoParams& video,
                                            std::span<const AudioParams> audio);
    [[nodiscard]] bool write_video_info(int fps);
    [[nodiscard]] bool write_syncpoint(uint32_t stream, int64_t pts);
    [[nodiscard]] uint32_t time_base_for(int den);

    ByteSink sink_;
    std::vector<int> time_base_dens_;
    std::vector<StreamInfo> streams_;
    bool header_written_ = false;
    bool syncpoint_written_ = false;
};

} // namespace nut
