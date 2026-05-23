// dma-probe — slice 1 verifier for the dma_heap allocator.
//
// Allocates a buffer from /dev/dma_heap/system, mmaps it, writes a known
// pattern, then reads it back to confirm round-trip. Also exercises the
// dma-buf sync ioctls so we catch any "missing CAP" type kernel errors early.
//
// Usage: ./dma-probe [size_bytes] [heap_name]

#include "src/ipc/dma_heap.hpp"

#include <cstdint>
#include <cstdio>
#include <cstdlib>
#include <cstring>

int main(int argc, char** argv) {
    size_t size = (argc > 1) ? std::strtoull(argv[1], nullptr, 0)
                             : (1920ULL * 1080ULL * 3ULL / 2ULL); // NV12 1080p
    std::string heap = (argc > 2) ? argv[2] : "system";

    auto buf = dmaheap::alloc(heap, size);
    if (!buf.valid()) {
        fprintf(stderr, "FAIL alloc heap=%s size=%zu\n", heap.c_str(), size);
        return 1;
    }
    printf("ok: alloc heap=%s size=%zu fd=%d\n", heap.c_str(), size, buf.fd.get());

    void* mapped = dmaheap::mmap_rw(buf);
    if (!mapped) {
        fprintf(stderr, "FAIL mmap\n");
        return 1;
    }
    printf("ok: mmap fd=%d size=%zu addr=%p\n", buf.fd.get(), buf.size, mapped);

    // Write a checkered pattern (alternating 0xAA / 0x55 bytes) inside a sync.
    dmaheap::sync_start(buf.fd.get(), dmaheap::SyncDir::Write);
    auto* p = static_cast<uint8_t*>(mapped);
    for (size_t i = 0; i < buf.size; ++i)
        p[i] = (i & 1) ? 0x55 : 0xAA;
    dmaheap::sync_end(buf.fd.get(), dmaheap::SyncDir::Write);
    printf("ok: wrote %zu bytes pattern\n", buf.size);

    // Read it back inside a sync; verify.
    dmaheap::sync_start(buf.fd.get(), dmaheap::SyncDir::Read);
    size_t mismatches = 0;
    for (size_t i = 0; i < buf.size; ++i) {
        uint8_t want = (i & 1) ? 0x55 : 0xAA;
        if (p[i] != want)
            ++mismatches;
    }
    dmaheap::sync_end(buf.fd.get(), dmaheap::SyncDir::Read);

    dmaheap::munmap_rw(mapped, buf.size);

    if (mismatches != 0) {
        fprintf(stderr, "FAIL: %zu/%zu bytes mismatched after round-trip\n", mismatches, buf.size);
        return 1;
    }
    printf("PASS: %zu bytes round-tripped via /dev/dma_heap/%s\n", buf.size, heap.c_str());
    return 0;
}
