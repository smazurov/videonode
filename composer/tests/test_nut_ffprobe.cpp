#include "src/sink/nut_writer.hpp"

#include <gtest/gtest.h>

#include <array>
#include <cstdio>
#include <cstdlib>
#include <fstream>
#include <string>
#include <unistd.h>
#include <vector>

namespace {

std::string run(const std::string& cmd) {
    std::string out;
    FILE* p = ::popen(cmd.c_str(), "r");
    if (p == nullptr)
        return out;
    std::array<char, 4096> buf{};
    size_t n = 0;
    while ((n = std::fread(buf.data(), 1, buf.size(), p)) > 0)
        out.append(buf.data(), n);
    ::pclose(p);
    return out;
}

bool have_ffprobe() {
    return std::system("ffprobe -version >/dev/null 2>&1") == 0;
}

std::string write_sample(int audio_tracks) {
    const std::string path = "/tmp/vn_nut_ffprobe_" + std::to_string(::getpid()) + "_" +
                             std::to_string(audio_tracks) + ".nut";
    std::ofstream f(path, std::ios::binary);
    nut::Writer w([&f](std::span<const uint8_t> b) {
        f.write(reinterpret_cast<const char*>(b.data()), static_cast<std::streamsize>(b.size()));
        return f.good();
    });
    const std::vector<nut::AudioParams> audio(audio_tracks, {.sample_rate = 48000, .channels = 2});
    EXPECT_TRUE(w.write_header({.width = 64, .height = 36, .fps = 30}, audio));
    const std::vector<uint8_t> frame(64 * 36 * 3 / 2, 128);
    const std::vector<uint8_t> pcm(1600 * 4, 0);
    for (int64_t i = 0; i < 30; ++i) {
        for (int k = 0; k < audio_tracks; ++k)
            EXPECT_TRUE(
                w.write_frame(nut::Writer::audio_stream(static_cast<uint32_t>(k)), i * 1600, pcm));
        EXPECT_TRUE(w.write_frame(nut::Writer::video_stream(), i, frame));
    }
    return path;
}

} // namespace

TEST(NutFfprobe, StreamsDecodeWithExpectedParameters) {
    if (!have_ffprobe())
        GTEST_SKIP() << "ffprobe not installed";
    const std::string path = write_sample(2);
    const std::string streams =
        run("ffprobe -v error -show_entries "
            "stream=codec_name,pix_fmt,width,height,sample_rate,channels,r_frame_rate -of csv=p=0 "
            "-f nut -i pipe:0 < " +
            path);
    EXPECT_NE(streams.find("rawvideo,64,36,nv12,30/1"), std::string::npos) << streams;
    EXPECT_NE(streams.find("pcm_s16le,48000,2"), std::string::npos) << streams;
    const std::string errors =
        run("ffmpeg -v error -f nut -i pipe:0 -map 0 -f null - 2>&1 < " + path);
    EXPECT_TRUE(errors.empty()) << errors;
    ::unlink(path.c_str());
}

TEST(NutFfprobe, PacketTimestampsRoundTrip) {
    if (!have_ffprobe())
        GTEST_SKIP() << "ffprobe not installed";
    const std::string path = write_sample(1);
    const std::string pkts = run(
        "ffprobe -v error -show_entries packet=stream_index,pts -of csv=p=0 -f nut -i pipe:0 < " +
        path);
    EXPECT_NE(pkts.find("0,29\n"), std::string::npos) << pkts;
    EXPECT_NE(pkts.find("1,46400\n"), std::string::npos) << pkts;
    int lines = 0;
    for (char c : pkts)
        lines += c == '\n' ? 1 : 0;
    EXPECT_EQ(lines, 60);
    ::unlink(path.c_str());
}

TEST(NutFfprobe, VideoOnlyStreamDecodes) {
    if (!have_ffprobe())
        GTEST_SKIP() << "ffprobe not installed";
    const std::string path = write_sample(0);
    const std::string errors =
        run("ffmpeg -v error -f nut -i pipe:0 -map 0 -f null - 2>&1 < " + path);
    EXPECT_TRUE(errors.empty()) << errors;
    ::unlink(path.c_str());
}
