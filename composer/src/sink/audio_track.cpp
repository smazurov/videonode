#include "src/sink/audio_track.hpp"

#include <algorithm>
#include <cmath>
#include <cstring>

namespace sink {

namespace {
constexpr int64_t kNsPerSec = 1'000'000'000;
}

AudioTrack::AudioTrack(const AudioTrackParams& p) : p_(p), clock_(p.sample_rate) {}

void AudioTrack::start(int64_t t0_ns) {
    t0_ns_ = t0_ns;
    out_pts_ = 0;
    last_pts_ = -1;
    started_ = true;
}

int64_t AudioTrack::frames_of(const Pending& p) const {
    return static_cast<int64_t>(p.samples.size()) / p_.channels;
}

int64_t AudioTrack::pts_for_ns(int64_t ns) const {
    const auto rel = static_cast<long double>(ns - t0_ns_ + p_.offset_ns);
    return std::llround(rel * p_.sample_rate / kNsPerSec);
}

void AudioTrack::push(std::span<const int16_t> samples, int64_t end_ns, bool discontinuity) {
    const int64_t frames = static_cast<int64_t>(samples.size()) / p_.channels;
    if (frames <= 0)
        return;
    if (discontinuity)
        clock_.reset();
    const int64_t start_ns = clock_.on_chunk(frames, end_ns);
    if (!started_)
        return;
    pending_.push_back({.pts = pts_for_ns(start_ns),
                        .samples = std::vector<int16_t>(samples.begin(), samples.end())});
    pending_frames_ += frames;
    enforce_pending_cap();
}

void AudioTrack::enforce_pending_cap() {
    while (pending_frames_ > p_.max_pending_frames && !pending_.empty()) {
        const int64_t n = frames_of(pending_.front());
        dropped_frames_ += n;
        pending_frames_ -= n;
        pending_.pop_front();
    }
}

void AudioTrack::trim_head(Pending& c, int64_t frames, bool late) {
    const auto samples = static_cast<std::ptrdiff_t>(frames * p_.channels);
    c.samples.erase(c.samples.begin(), c.samples.begin() + samples);
    c.pts += frames;
    if (late)
        dropped_frames_ += frames;
    pending_frames_ -= frames;
}

void AudioTrack::emit_silence(int64_t until_pts, std::vector<AudioPacket>& out) {
    while (out_pts_ < until_pts) {
        const int64_t n = std::min(p_.silence_packet_frames, until_pts - out_pts_);
        const auto bytes = static_cast<size_t>(n * p_.channels) * sizeof(int16_t);
        out.push_back({.pts = out_pts_, .data = std::vector<uint8_t>(bytes, 0)});
        last_pts_ = out_pts_;
        out_pts_ += n;
        silence_frames_ += n;
    }
}

void AudioTrack::emit_chunk(Pending& c, std::vector<AudioPacket>& out) {
    const int64_t n = frames_of(c);
    const int64_t pts = std::max(c.pts, last_pts_ + 1);
    std::vector<uint8_t> bytes(c.samples.size() * sizeof(int16_t));
    std::memcpy(bytes.data(), c.samples.data(), bytes.size());
    out.push_back({.pts = pts, .data = std::move(bytes)});
    last_pts_ = pts;
    out_pts_ = pts + n;
    pending_frames_ -= n;
}

void AudioTrack::drain(int64_t now_ns, std::vector<AudioPacket>& out) {
    if (!started_)
        return;
    while (!pending_.empty()) {
        Pending& c = pending_.front();
        const int64_t n = frames_of(c);
        const bool preroll = c.pts + n <= 0;
        if (preroll || c.pts + n <= out_pts_ - p_.tolerance_frames) {
            if (!preroll)
                dropped_frames_ += n;
            pending_frames_ -= n;
            pending_.pop_front();
            continue;
        }
        if (c.pts < 0)
            trim_head(c, -c.pts, false);
        const int64_t floor_pts = std::max<int64_t>(0, out_pts_ - p_.tolerance_frames);
        if (c.pts < floor_pts)
            trim_head(c, floor_pts - c.pts, true);
        if (c.pts > out_pts_ + p_.tolerance_frames)
            emit_silence(c.pts, out);
        emit_chunk(c, out);
        pending_.pop_front();
    }
    emit_silence(pts_for_ns(now_ns - p_.fill_after_ns), out);
}

} // namespace sink
