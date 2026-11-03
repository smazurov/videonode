#include "src/sink/nut_writer.hpp"

#include <array>
#include <cstdlib>
#include <string>
#include <string_view>

namespace nut {

namespace {

constexpr uint64_t kMainStartcode = 0x4E4D7A561F5F04ADULL;
constexpr uint64_t kStreamStartcode = 0x4E5311405BF2F9DBULL;
constexpr uint64_t kSyncpointStartcode = 0x4E4BE4ADEECA4569ULL;
constexpr uint64_t kInfoStartcode = 0x4E49AB68B596BA78ULL;

constexpr uint64_t kVersion = 4;
constexpr uint64_t kMinorVersion = 1;
constexpr uint64_t kMaxDistance = 32767;
constexpr uint64_t kFlagPipe = 2;
constexpr uint64_t kPtsShift = 7;
constexpr size_t kHeaderChecksumThreshold = 4096;

constexpr uint64_t kFlagKey = 1;
constexpr uint64_t kFlagCodedPts = 8;
constexpr uint64_t kFlagStreamId = 16;
constexpr uint64_t kFlagSizeMsb = 32;
constexpr uint64_t kFlagChecksum = 64;
constexpr uint64_t kFlagCoded = 4096;
constexpr uint64_t kFrameFlags =
    kFlagKey | kFlagCodedPts | kFlagStreamId | kFlagSizeMsb | kFlagChecksum;

constexpr std::string_view kIdString = "nut/multimedia container";
constexpr std::array<uint8_t, 4> kFourccNv12{'N', 'V', '1', '2'};
constexpr std::array<uint8_t, 4> kFourccPcmS16le{'P', 'S', 'D', 16};

std::array<uint32_t, 256> make_crc_table() {
    std::array<uint32_t, 256> t{};
    for (uint32_t i = 0; i < 256; ++i) {
        uint32_t c = i << 24;
        for (int b = 0; b < 8; ++b)
            c = (c & 0x80000000U) != 0 ? (c << 1) ^ 0x04C11DB7U : c << 1;
        t.at(i) = c;
    }
    return t;
}

void put_be32(std::vector<uint8_t>& out, uint32_t v) {
    out.push_back(static_cast<uint8_t>(v >> 24));
    out.push_back(static_cast<uint8_t>(v >> 16));
    out.push_back(static_cast<uint8_t>(v >> 8));
    out.push_back(static_cast<uint8_t>(v));
}

void put_be64(std::vector<uint8_t>& out, uint64_t v) {
    put_be32(out, static_cast<uint32_t>(v >> 32));
    put_be32(out, static_cast<uint32_t>(v));
}

void put_bytes(std::vector<uint8_t>& out, std::span<const uint8_t> b) {
    out.insert(out.end(), b.begin(), b.end());
}

void put_str(std::vector<uint8_t>& out, std::string_view s) {
    put_v(out, s.size());
    out.insert(out.end(), s.begin(), s.end());
}

} // namespace

uint32_t crc32(uint32_t crc, std::span<const uint8_t> bytes) {
    static const std::array<uint32_t, 256> table = make_crc_table();
    for (uint8_t b : bytes)
        crc = (crc << 8) ^ table.at(((crc >> 24) ^ b) & 0xFFU);
    return crc;
}

void put_v(std::vector<uint8_t>& out, uint64_t v) {
    int len = 1;
    for (uint64_t t = v >> 7; t != 0; t >>= 7)
        ++len;
    for (int i = len - 1; i > 0; --i)
        out.push_back(static_cast<uint8_t>(0x80U | ((v >> (7 * i)) & 0x7FU)));
    out.push_back(static_cast<uint8_t>(v & 0x7FU));
}

void put_s(std::vector<uint8_t>& out, int64_t v) {
    const uint64_t mag = static_cast<uint64_t>(std::llabs(v));
    put_v(out, 2 * mag - (v > 0 ? 1 : 0));
}

bool Writer::put_packet(uint64_t startcode, std::span<const uint8_t> payload) {
    std::vector<uint8_t> out;
    out.reserve(payload.size() + 24);
    put_be64(out, startcode);
    const uint64_t forward = payload.size() + 4;
    put_v(out, forward);
    if (forward > kHeaderChecksumThreshold)
        put_be32(out, crc32(0, out));
    put_bytes(out, payload);
    put_be32(out, crc32(0, payload));
    return sink_(out);
}

uint32_t Writer::time_base_for(int den) {
    for (size_t i = 0; i < time_base_dens_.size(); ++i) {
        if (time_base_dens_[i] == den)
            return static_cast<uint32_t>(i);
    }
    time_base_dens_.push_back(den);
    return static_cast<uint32_t>(time_base_dens_.size() - 1);
}

bool Writer::write_main_header() {
    std::vector<uint8_t> p;
    put_v(p, kVersion);
    put_v(p, kMinorVersion);
    put_v(p, streams_.size());
    put_v(p, kMaxDistance);
    put_v(p, time_base_dens_.size());
    for (int den : time_base_dens_) {
        put_v(p, 1);
        put_v(p, static_cast<uint64_t>(den));
    }
    // One run of 255 frame codes (everything but 'N'), all fully coded per frame.
    put_v(p, kFlagCoded);
    put_v(p, 6);
    put_s(p, 0);
    put_v(p, 1);
    put_v(p, 0);
    put_v(p, 0);
    put_v(p, 0);
    put_v(p, 255);
    put_v(p, 0);
    put_v(p, kFlagPipe);
    return put_packet(kMainStartcode, p);
}

bool Writer::write_stream_headers(const VideoParams& video, std::span<const AudioParams> audio) {
    std::vector<uint8_t> p;
    put_v(p, video_stream());
    put_v(p, 0);
    put_v(p, kFourccNv12.size());
    put_bytes(p, kFourccNv12);
    put_v(p, streams_[video_stream()].time_base_id);
    put_v(p, kPtsShift);
    put_v(p, static_cast<uint64_t>(video.fps));
    put_v(p, 0);
    put_v(p, 0);
    put_v(p, 0);
    put_v(p, static_cast<uint64_t>(video.width));
    put_v(p, static_cast<uint64_t>(video.height));
    put_v(p, 1);
    put_v(p, 1);
    put_v(p, 0);
    if (!put_packet(kStreamStartcode, p))
        return false;

    for (uint32_t k = 0; k < audio.size(); ++k) {
        const AudioParams& a = audio[k];
        p.clear();
        put_v(p, audio_stream(k));
        put_v(p, 1);
        put_v(p, kFourccPcmS16le.size());
        put_bytes(p, kFourccPcmS16le);
        put_v(p, streams_[audio_stream(k)].time_base_id);
        put_v(p, kPtsShift);
        put_v(p, static_cast<uint64_t>(a.sample_rate));
        put_v(p, 0);
        put_v(p, 0);
        put_v(p, 0);
        put_v(p, static_cast<uint64_t>(a.sample_rate));
        put_v(p, 1);
        put_v(p, static_cast<uint64_t>(a.channels));
        if (!put_packet(kStreamStartcode, p))
            return false;
    }
    return true;
}

bool Writer::write_video_info(int fps) {
    std::vector<uint8_t> p;
    put_v(p, video_stream() + 1);
    put_s(p, 0);
    put_v(p, 0);
    put_v(p, 0);
    put_v(p, 1);
    put_str(p, "r_frame_rate");
    put_s(p, -1);
    put_str(p, std::to_string(fps) + "/1");
    return put_packet(kInfoStartcode, p);
}

bool Writer::write_header(const VideoParams& video, std::span<const AudioParams> audio) {
    if (header_written_ || video.width <= 0 || video.height <= 0 || video.fps <= 0)
        return false;
    for (const AudioParams& a : audio) {
        if (a.sample_rate <= 0 || a.channels <= 0)
            return false;
    }
    streams_.clear();
    time_base_dens_.clear();
    streams_.push_back({.time_base_id = time_base_for(video.fps)});
    for (const AudioParams& a : audio)
        streams_.push_back({.time_base_id = time_base_for(a.sample_rate)});

    std::vector<uint8_t> id(kIdString.begin(), kIdString.end());
    id.push_back(0);
    if (!sink_(id))
        return false;
    if (!write_main_header() || !write_stream_headers(video, audio) || !write_video_info(video.fps))
        return false;
    header_written_ = true;
    return true;
}

bool Writer::write_syncpoint(uint32_t stream, int64_t pts) {
    std::vector<uint8_t> p;
    const uint64_t tb_count = time_base_dens_.size();
    put_v(p, static_cast<uint64_t>(pts) * tb_count + streams_[stream].time_base_id);
    put_v(p, 0);
    return put_packet(kSyncpointStartcode, p);
}

bool Writer::write_frame(uint32_t stream, int64_t pts, std::span<const uint8_t> data) {
    if (!header_written_ || stream >= streams_.size() || pts < 0)
        return false;
    if (!syncpoint_written_) {
        if (!write_syncpoint(stream, pts))
            return false;
        syncpoint_written_ = true;
    }
    std::vector<uint8_t> h;
    h.reserve(24);
    h.push_back(0);
    put_v(h, kFrameFlags);
    put_v(h, stream);
    put_v(h, static_cast<uint64_t>(pts) + (1ULL << kPtsShift));
    put_v(h, data.size());
    put_be32(h, crc32(0, h));
    return sink_(h) && (data.empty() || sink_(data));
}

} // namespace nut
