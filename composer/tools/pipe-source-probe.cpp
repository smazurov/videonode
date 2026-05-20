// pipe-source-probe — verifier for FfmpegPipeSource.
//
// Captures N frames from a V4L2 device via ffmpeg subprocess; dumps the last
// captured frame to a Y4M file we can ffplay back on the dev machine.
//
// Usage: ./pipe-source-probe [device] [input_format] [W] [H] [fps] [seconds] [out.y4m]
//   - Lyra MJPEG @1080p30 for 3s:
//       ./pipe-source-probe /dev/video1 mjpeg 1920 1080 30 3 /tmp/lyra.y4m
//   - HDMI NV12 @4K30 for 3s:
//       ./pipe-source-probe /dev/video0 nv12 3840 2160 30 3 /tmp/hdmi.y4m

#include "../src/ffmpeg_pipe_source.hpp"
#include "../src/dma_heap.hpp"

#include <chrono>
#include <cstdint>
#include <cstdio>
#include <cstdlib>
#include <cstring>
#include <sys/mman.h>
#include <thread>
#include <unistd.h>

int main(int argc, char** argv) {
    using namespace ffmpeg_pipe_source;
    InitParams p;
    p.device = (argc > 1) ? argv[1] : "/dev/video1";
    p.input_format = (argc > 2) ? argv[2] : "mjpeg";
    p.width = (argc > 3) ? std::atoi(argv[3]) : 1920;
    p.height = (argc > 4) ? std::atoi(argv[4]) : 1080;
    p.fps = (argc > 5) ? std::atoi(argv[5]) : 30;
    int seconds = (argc > 6) ? std::atoi(argv[6]) : 3;
    const char* out = (argc > 7) ? argv[7] : "/tmp/pipe-source.y4m";

    FfmpegPipeSource src;
    if (!src.init(p)) {
        fprintf(stderr, "FAIL init\n");
        return 1;
    }
    if (!src.start()) {
        fprintf(stderr, "FAIL start\n");
        return 1;
    }
    fprintf(stderr, "ok: started ffmpeg pid=%d\n", src.pid());

    // Wait up to (seconds + 2) for the first frame to appear, then run for
    // `seconds` seconds, periodically reading the latest frame.
    auto deadline = std::chrono::steady_clock::now() + std::chrono::seconds(seconds + 5);
    uint64_t first_idx = 0;
    while (std::chrono::steady_clock::now() < deadline) {
        auto v = src.latest_frame();
        if (v.frame_idx > 0) {
            first_idx = v.frame_idx;
            fprintf(stderr, "ok: first frame idx=%lu\n", (unsigned long)v.frame_idx);
            break;
        }
        std::this_thread::sleep_for(std::chrono::milliseconds(50));
    }
    if (first_idx == 0) {
        src.stop();
        fprintf(stderr, "FAIL: no frames in %ds\n", seconds + 5);
        return 1;
    }

    // Sit and tick for `seconds`, then capture the most recent frame.
    auto end = std::chrono::steady_clock::now() + std::chrono::seconds(seconds);
    uint64_t last_idx = 0;
    while (std::chrono::steady_clock::now() < end) {
        auto v = src.latest_frame();
        last_idx = v.frame_idx;
        std::this_thread::sleep_for(std::chrono::milliseconds(33));
    }
    fprintf(stderr, "ok: %lu frames received\n", (unsigned long)(last_idx - first_idx + 1));

    // Grab the final frame and dump as Y4M for visual verification.
    auto fv = src.latest_frame();
    if (fv.fd < 0) {
        fprintf(stderr, "FAIL: no frame fd\n");
        src.stop();
        return 1;
    }
    size_t bytes = static_cast<size_t>(fv.width) * fv.height * 3 / 2;
    void* p_map = ::mmap(nullptr, bytes, PROT_READ, MAP_SHARED, fv.fd, 0);
    if (p_map == MAP_FAILED) {
        fprintf(stderr, "FAIL mmap latest\n");
        src.stop();
        return 1;
    }

    FILE* f = std::fopen(out, "wb");
    if (!f) {
        fprintf(stderr, "FAIL fopen %s\n", out);
        src.stop();
        return 1;
    }
    std::fprintf(f, "YUV4MPEG2 W%d H%d F%d:1 Ip A1:1 C420mpeg2\n", fv.width, fv.height, p.fps);
    std::fprintf(f, "FRAME\n");
    auto* pb = static_cast<uint8_t*>(p_map);
    size_t y_size = static_cast<size_t>(fv.width) * fv.height;
    size_t uv_size = y_size / 2;
    std::fwrite(pb, 1, y_size, f); // Y plane straight through
    auto* uv = pb + y_size;
    for (size_t i = 0; i < uv_size; i += 2)
        std::fputc(uv[i], f);
    for (size_t i = 0; i < uv_size; i += 2)
        std::fputc(uv[i + 1], f);
    std::fclose(f);
    ::munmap(p_map, bytes);

    src.stop();
    printf("PASS: dumped frame %lu (%dx%d) to %s\n", (unsigned long)fv.frame_idx, fv.width,
           fv.height, out);
    return 0;
}
