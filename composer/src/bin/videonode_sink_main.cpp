// videonode-sink — SCM_RIGHTS frame consumer feeding ffmpeg on stdout.
//
//   --mux raw (default): raw NV12 per producer frame;
//     ffmpeg -f rawvideo -pix_fmt nv12 -video_size WxH -framerate N -i pipe:0
//   --mux nut: paced NV12 + one PCM track per --audio-device in a NUT stream;
//     ffmpeg -f nut -i pipe:0

#include "src/common/log_levels.hpp"
#include "src/common/raise_fd_limit.hpp"
#include "src/common/signal.hpp"
#include "src/sink/nut_session.hpp"
#include "src/sink/raw_session.hpp"
#include "version.hpp"

#include <atomic>
#include <cstdio>
#include <cstdlib>
#include <span>
#include <string>

namespace {

std::atomic<bool> g_running{true};

struct Args {
    std::string mux = "raw";
    sink::RawSessionArgs raw;
    sink::NutSessionArgs nut;
};

void print_help() {
    const sink::RawSessionArgs r;
    const sink::NutSessionArgs n;
    fprintf(stderr,
            "videonode-sink — stream frames from an SCM_RIGHTS socket to stdout.\n"
            "\n"
            "  --socket PATH                Unix socket exposed by videonode-source OR\n"
            "                                 a videonode-composer --scm-out (required)\n"
            "  --mux raw|nut                output container (default raw)\n"
            "  -v, --verbose                log per-frame frame_idx to stderr\n"
            "  --poll-ms N                  raw: sleep between consumer polls (default %d)\n"
            "  --settle-ms N                delay after dial before reading (default %d)\n"
            "  --first-frame-timeout N      seconds to wait for first frame (default %d)\n"
            "\n"
            "nut only:\n"
            "  --fps N                      video slot rate; gaps repeat the last frame (default "
            "%d)\n"
            "  --audio-device NAME          ALSA capture PCM; repeat for more tracks\n"
            "  --audio-rate N               capture rate in Hz (default %d)\n"
            "  --audio-channels N           capture channels (default %d)\n"
            "  --audio-offset-ms N          shift audio vs video; positive delays audio\n"
            "\n"
            "raw emits NV12 bytes (Y plane, then UV plane) per frame:\n"
            "  ffmpeg -f rawvideo -pix_fmt nv12 -video_size WxH -framerate N -i pipe:0\n"
            "nut emits a self-describing stream: ffmpeg -f nut -i pipe:0\n",
            r.poll_ms, r.settle_ms, r.first_frame_timeout_s, n.fps, n.audio_rate, n.audio_channels);
}

[[nodiscard]] bool apply_int_flag(const std::string& flag, int value, Args& a) {
    if (flag == "--poll-ms")
        a.raw.poll_ms = value;
    else if (flag == "--settle-ms")
        a.raw.settle_ms = a.nut.settle_ms = value;
    else if (flag == "--first-frame-timeout")
        a.raw.first_frame_timeout_s = a.nut.first_frame_timeout_s = value;
    else if (flag == "--fps")
        a.nut.fps = value;
    else if (flag == "--audio-rate")
        a.nut.audio_rate = value;
    else if (flag == "--audio-channels")
        a.nut.audio_channels = value;
    else if (flag == "--audio-offset-ms")
        a.nut.audio_offset_ms = value;
    else
        return false;
    return true;
}

[[nodiscard]] bool apply_string_flag(const std::string& flag, const std::string& value, Args& a) {
    if (flag == "--socket")
        a.raw.socket_path = a.nut.socket_path = value;
    else if (flag == "--mux")
        a.mux = value;
    else if (flag == "--audio-device")
        a.nut.audio_devices.push_back(value);
    else
        return false;
    return true;
}

// Returns -1 on parse error, 0 on success, 1 when --help/--version was handled.
int parse_args(int argc, char** argv, Args& a) {
    std::span<char*> args(argv, static_cast<size_t>(argc));
    for (size_t i = 1; i < args.size(); ++i) {
        const std::string s = args[i];
        if (s == "-h" || s == "--help") {
            print_help();
            return 1;
        }
        if (s == "--version") {
            printf("videonode-sink %s\n", vn::kVersion);
            return 1;
        }
        if (s == "-v" || s == "--verbose") {
            a.raw.verbose = a.nut.verbose = true;
            continue;
        }
        const std::string value = i + 1 < args.size() ? args[i + 1] : "";
        if (apply_string_flag(s, value, a) || apply_int_flag(s, std::atoi(value.c_str()), a)) {
            ++i;
            continue;
        }
        vn::log::error("videonode-sink: unknown arg %s (use --help)", s.c_str());
        return -1;
    }
    if (a.raw.socket_path.empty()) {
        vn::log::error("videonode-sink: --socket PATH is required");
        return -1;
    }
    if (a.mux != "raw" && a.mux != "nut") {
        vn::log::error("videonode-sink: --mux must be raw or nut");
        return -1;
    }
    return 0;
}

} // namespace

int main(int argc, char** argv) {
    vn::raise_fd_limit();

    Args a;
    const int parse_result = parse_args(argc, argv, a);
    if (parse_result != 0)
        return parse_result > 0 ? 0 : 2;

    ::setvbuf(stderr, nullptr, _IOLBF, 0);
    vn::signal::install_shutdown(g_running);

    const int rc = a.mux == "nut" ? sink::run_nut_session(a.nut, g_running)
                                  : sink::run_raw_session(a.raw, g_running);
    vn::log::info("videonode-sink: shutdown");
    return rc;
}
