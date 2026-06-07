// credit-stall-probe — rig-only faithful repro for the source ring-overwrite
// tear. Connects to a videonode-source data socket, holds one ring slot WITHOUT
// crediting it, credits every other slot so the producer keeps cycling the ring,
// then checks whether the held slot's luma bytes were overwritten. With the
// per-slot in-flight gate the held slot must survive (producer drops instead);
// without it the producer recycles the slot mid-hold and the bytes change.

#include "src/common/unique_fd.hpp"
#include "src/ipc/dmabuf_header.hpp"
#include "src/ipc/scm_socket.hpp"

#include <cerrno>
#include <cstdint>
#include <cstdio>
#include <cstring>
#include <linux/dma-buf.h>
#include <span>
#include <string>
#include <sys/ioctl.h>
#include <sys/mman.h>
#include <unistd.h>
#include <vector>

namespace {

using vn::base::unique_fd;

constexpr uint64_t kSentinel = 0xFFFFFFFFu;

void dmabuf_sync(int fd, uint64_t flags) {
    dma_buf_sync sync = {};
    sync.flags = flags;
    while (::ioctl(fd, DMA_BUF_IOCTL_SYNC, &sync) == -1 && (errno == EINTR || errno == EAGAIN)) {
    }
}

std::vector<uint8_t> read_luma(int fd, size_t offset, size_t bytes) {
    const off_t total = ::lseek(fd, 0, SEEK_END);
    if (total <= 0)
        return {};
    void* p = ::mmap(nullptr, static_cast<size_t>(total), PROT_READ, MAP_SHARED, fd, 0);
    if (p == MAP_FAILED)
        return {};
    dmabuf_sync(fd, DMA_BUF_SYNC_START | DMA_BUF_SYNC_READ);
    std::span<const uint8_t> base(static_cast<const uint8_t*>(p), static_cast<size_t>(total));
    std::vector<uint8_t> out(base.subspan(offset, bytes).begin(),
                             base.subspan(offset, bytes).end());
    dmabuf_sync(fd, DMA_BUF_SYNC_END | DMA_BUF_SYNC_READ);
    ::munmap(p, static_cast<size_t>(total));
    return out;
}

void close_all(std::vector<int>& fds) {
    for (int f : fds)
        if (f >= 0)
            ::close(f);
    fds.clear();
}

struct Held {
    unique_fd fd;
    uint64_t slot = kSentinel;
    uint64_t generation = 0;
    size_t offset = 0;
    size_t bytes = 0;
    std::vector<uint8_t> snapshot;
};

bool grab_held(int sock, Held& h) {
    for (;;) {
        dmabuf_header::Header hdr;
        std::vector<int> fds;
        bool eof = false;
        if (!scm_socket::RecvMessage(sock, hdr, fds, &eof)) {
            if (eof)
                std::fprintf(stderr, "credit-stall-probe: source closed before a ring frame\n");
            return false;
        }
        if (hdr.slot_index == kSentinel || fds.empty()) {
            close_all(fds);
            continue;
        }
        h.slot = hdr.slot_index;
        h.generation = hdr.generation;
        h.offset = hdr.plane_offsets.empty() ? 0 : hdr.plane_offsets[0];
        h.bytes =
            static_cast<size_t>(hdr.plane_pitches.empty() ? hdr.width : hdr.plane_pitches[0]) *
            hdr.height;
        h.fd = unique_fd(::dup(fds[0]));
        close_all(fds);
        h.snapshot = read_luma(h.fd.get(), h.offset, h.bytes);
        return !h.snapshot.empty();
    }
}

} // namespace

int main(int argc, char** argv) {
    std::string socket_path;
    int frames = 64;
    for (int i = 1; i < argc; ++i) {
        std::string a = argv[i];
        if (a == "--socket" && i + 1 < argc)
            socket_path = argv[++i];
        else if (a == "--frames" && i + 1 < argc)
            frames = std::atoi(argv[++i]);
    }
    if (socket_path.empty()) {
        std::fprintf(stderr, "usage: credit-stall-probe --socket <path> [--frames N]\n");
        return 2;
    }

    unique_fd sock = scm_socket::ConnectClient(socket_path);
    if (!sock || !scm_socket::SendReady(sock.get())) {
        std::fprintf(stderr, "credit-stall-probe: connect/handshake failed: %s\n", strerror(errno));
        return 1;
    }

    Held held;
    if (!grab_held(sock.get(), held)) {
        std::fprintf(stderr, "credit-stall-probe: failed to grab a ring frame\n");
        return 1;
    }
    std::fprintf(stderr, "credit-stall-probe: holding slot=%llu gen=%llu (%zu luma bytes)\n",
                 static_cast<unsigned long long>(held.slot),
                 static_cast<unsigned long long>(held.generation), held.bytes);

    int reused = 0;
    for (int i = 0; i < frames; ++i) {
        dmabuf_header::Header hdr;
        std::vector<int> fds;
        bool eof = false;
        if (!scm_socket::RecvMessage(sock.get(), hdr, fds, &eof)) {
            if (eof)
                break;
            continue;
        }
        if (hdr.slot_index == held.slot && hdr.generation != held.generation)
            ++reused;
        else if (hdr.slot_index != held.slot && hdr.slot_index != kSentinel)
            (void)scm_socket::SendCredit(
                sock.get(), {.slot_index = hdr.slot_index, .generation = hdr.generation});
        close_all(fds);
    }

    std::vector<uint8_t> after = read_luma(held.fd.get(), held.offset, held.bytes);
    const bool changed = after != held.snapshot;
    const bool fail = changed || reused > 0;
    std::fprintf(stderr, "credit-stall-probe: %s — held-slot reuse=%d, luma %s\n",
                 fail ? "FAIL (overwrite)" : "PASS (slot protected)", reused,
                 changed ? "CHANGED" : "stable");
    return fail ? 1 : 0;
}
