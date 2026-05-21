// source-probe — slice 2 verifier for fake_source.
//
// Creates 4 FakeSources at a chosen resolution, ticks each one a few times,
// then dumps frame N from source K as a YUV4MPEG file we can play with ffmpeg
// on the dev machine.
//
// Usage:  ./source-probe [W] [H] [frames] [source_index] [out.y4m]

#include "src/render/fake_source.hpp"
#include "src/ipc/dma_heap.hpp"

#include <cstdint>
#include <cstdio>
#include <cstdlib>
#include <cstring>
#include <unistd.h>
#include <sys/mman.h>

int main(int argc, char** argv) {
    int w = (argc > 1) ? std::atoi(argv[1]) : 640;
    int h = (argc > 2) ? std::atoi(argv[2]) : 480;
    int frames = (argc > 3) ? std::atoi(argv[3]) : 30;
    int pick_src = (argc > 4) ? std::atoi(argv[4]) : 0;
    const char* out = (argc > 5) ? argv[5] : "/tmp/source-probe.y4m";

    using fake_source::FakeSource;
    FakeSource src[4];
    fake_source::Color colors[4] = {fake_source::kRed, fake_source::kGreen, fake_source::kBlue,
                                    fake_source::kYellow};
    for (int i = 0; i < 4; ++i) {
        if (!src[i].init(w, h, colors[i])) {
            fprintf(stderr, "FAIL init source %d\n", i);
            return 1;
        }
        printf("ok: source %d wxh=%dx%d fd=%d size=%zu\n", i, w, h, src[i].dmabuf_fd(),
               src[i].size());
    }

    if (pick_src < 0 || pick_src >= 4) {
        fprintf(stderr, "FAIL: pick_src=%d out of range\n", pick_src);
        return 1;
    }

    // YUV4MPEG (Y4M) header + frame loop dumped from source[pick_src].
    FILE* f = std::fopen(out, "wb");
    if (!f) {
        fprintf(stderr, "FAIL fopen(%s)\n", out);
        return 1;
    }
    std::fprintf(f, "YUV4MPEG2 W%d H%d F30:1 Ip A1:1 C420mpeg2\n", w, h);

    auto* p = reinterpret_cast<uint8_t*>(
        ::mmap(nullptr, src[pick_src].size(), PROT_READ, MAP_SHARED, src[pick_src].dmabuf_fd(), 0));
    if (p == MAP_FAILED) {
        fprintf(stderr, "FAIL mmap for dump\n");
        return 1;
    }

    for (int i = 0; i < frames; ++i) {
        for (int k = 0; k < 4; ++k)
            src[k].tick(i); // tick all so they progress independently
        dmaheap::sync_start(src[pick_src].dmabuf_fd(), dmaheap::SyncDir::Read);
        std::fputs("FRAME\n", f);
        // NV12 -> I420 conversion: Y plane is the same; deinterleave UV into U then V.
        size_t y_size = static_cast<size_t>(w) * h;
        size_t uv_size = y_size / 2;
        std::fwrite(p, 1, y_size, f);
        // Write U plane (every other byte starting at 0) then V plane (every other byte starting at
        // 1).
        auto* uv = p + y_size;
        for (size_t j = 0; j < uv_size; j += 2)
            std::fputc(uv[j], f);
        for (size_t j = 0; j < uv_size; j += 2)
            std::fputc(uv[j + 1], f);
        dmaheap::sync_end(src[pick_src].dmabuf_fd(), dmaheap::SyncDir::Read);
    }

    ::munmap(p, src[pick_src].size());
    std::fclose(f);
    printf("PASS: wrote %d frames of source %d to %s\n", frames, pick_src, out);
    return 0;
}
