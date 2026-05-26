// fd lifetime tests for the SCM_RIGHTS pipeline.
//
// Validates that consumers borrowing raw fd ints from latest_frame() can
// be invalidated by the source advancing frames. These races exist in
// both the composer (build_render_slots_ → dup gap) and vn-sink (mmap
// on a borrowed fd).

#include "src/ipc/dmabuf_header.hpp"
#include "src/ipc/scm_rights_producer.hpp"
#include "src/ipc/scm_rights_source.hpp"
#include "src/ipc/scm_socket.hpp"

#include <gtest/gtest.h>

#include <chrono>
#include <cstring>
#include <sys/mman.h>
#include <thread>
#include <unistd.h>
#include <vector>

namespace {

using vn::base::unique_fd;

unique_fd make_memfd(size_t size) {
    int fd = static_cast<int>(::syscall(SYS_memfd_create, "test", MFD_CLOEXEC));
    if (fd < 0)
        return {};
    if (::ftruncate(fd, static_cast<off_t>(size)) < 0) {
        ::close(fd);
        return {};
    }
    return unique_fd(fd);
}

dmabuf_header::Header make_header(uint64_t frame_idx) {
    dmabuf_header::Header h;
    h.width = 64;
    h.height = 64;
    h.format = "NV12";
    h.plane_pitches = {64, 64};
    h.plane_offsets = {0, 0};
    h.frame_idx = frame_idx;
    return h;
}

} // namespace

// After 2 frames advance through a ScmRightsSource, the fds from the
// first frame are closed. A consumer holding borrowed raw int fds (as
// the composer and vn-sink do) can no longer dup() them.
TEST(FdLifetime, BorrowedFdsGoStaleAfterTwoFrames) {
    scm_rights_producer::ScmRightsProducer prod;
    scm_rights_producer::InitParams pp;
    std::string sock_path = "/tmp/test-fd-lifetime-" + std::to_string(getpid()) + ".sock";
    pp.socket_path = sock_path;
    ASSERT_TRUE(prod.init(pp));
    ASSERT_TRUE(prod.start());

    scm_rights_source::ScmRightsSource consumer;
    scm_rights_source::InitParams cp;
    cp.socket_path = sock_path;
    cp.dial = true;
    ASSERT_TRUE(consumer.init(cp));
    ASSERT_TRUE(consumer.start());

    std::this_thread::sleep_for(std::chrono::milliseconds(50));

    constexpr size_t kBufSize = 4096;
    unique_fd fd0 = make_memfd(kBufSize);
    unique_fd fd1 = make_memfd(kBufSize);
    unique_fd fd2 = make_memfd(kBufSize);
    unique_fd fd3 = make_memfd(kBufSize);
    ASSERT_TRUE(fd0);
    ASSERT_TRUE(fd1);

    // Frame 0: send two fds.
    prod.broadcast(make_header(1), {fd0.get(), fd1.get()});
    std::this_thread::sleep_for(std::chrono::milliseconds(20));

    // Consumer grabs frame 0's owned fds.
    auto v0 = consumer.latest_frame();
    ASSERT_EQ(v0.frame_idx, 1u);
    int owned_fd = v0.fd.get();
    ASSERT_GE(owned_fd, 0);

    // Verify the owned fd is valid right now.
    int duped = ::dup(owned_fd);
    ASSERT_GE(duped, 0) << "owned fd should be valid immediately after latest_frame()";
    ::close(duped);

    // Frame 1: advances latest, frame 0 moves to prev_fds_.
    unique_fd fd1b = make_memfd(kBufSize);
    unique_fd fd1c = make_memfd(kBufSize);
    prod.broadcast(make_header(2), {fd1b.get(), fd1c.get()});
    std::this_thread::sleep_for(std::chrono::milliseconds(20));

    // OwnedFrameView keeps its fds alive regardless of source state.
    duped = ::dup(owned_fd);
    EXPECT_GE(duped, 0) << "owned fd should survive one frame advance";
    if (duped >= 0)
        ::close(duped);

    // Frame 2: source drops prev_fds_, but our OwnedFrameView still holds.
    prod.broadcast(make_header(3), {fd2.get(), fd3.get()});
    std::this_thread::sleep_for(std::chrono::milliseconds(20));

    // With OwnedFrameView, the fd stays valid even after 2+ advances.
    duped = ::dup(owned_fd);
    EXPECT_GE(duped, 0) << "OwnedFrameView should keep fd alive after 2 frame advances";
    if (duped >= 0) {
        printf("  [OK] OwnedFrameView fd %d still valid after 2 frame advances — "
               "borrow gap eliminated\n",
               owned_fd);
        ::close(duped);
    }

    consumer.stop();
    prod.stop();
}

// A consumer that dup()s the fd immediately after latest_frame() is safe
// even if frames advance — the underlying dma-buf stays alive as long as
// any fd referencing it is open.
TEST(FdLifetime, DupBeforeUseKeepsDmabufAlive) {
    std::string sock_path = "/tmp/test-fd-dup-" + std::to_string(getpid()) + ".sock";

    scm_rights_producer::ScmRightsProducer prod;
    scm_rights_producer::InitParams pp;
    pp.socket_path = sock_path;
    ASSERT_TRUE(prod.init(pp));
    ASSERT_TRUE(prod.start());

    scm_rights_source::ScmRightsSource consumer;
    scm_rights_source::InitParams cp;
    cp.socket_path = sock_path;
    cp.dial = true;
    ASSERT_TRUE(consumer.init(cp));
    ASSERT_TRUE(consumer.start());

    std::this_thread::sleep_for(std::chrono::milliseconds(50));

    constexpr size_t kBufSize = 4096;
    unique_fd src_fd = make_memfd(kBufSize);
    ASSERT_TRUE(src_fd);

    // Write a marker into the memfd so we can verify we read the right buf.
    void* src_map = ::mmap(nullptr, kBufSize, PROT_WRITE, MAP_SHARED, src_fd.get(), 0);
    ASSERT_NE(src_map, MAP_FAILED);
    std::memset(src_map, 0xAB, kBufSize);
    ::munmap(src_map, kBufSize);

    unique_fd src_fd2 = make_memfd(kBufSize);
    prod.broadcast(make_header(1), {src_fd.get(), src_fd2.get()});
    std::this_thread::sleep_for(std::chrono::milliseconds(20));

    auto v = consumer.latest_frame();
    ASSERT_EQ(v.frame_idx, 1u);
    ASSERT_TRUE(v.fd);

    // Advance 2 frames to invalidate the source's copy.
    for (int i = 2; i <= 3; ++i) {
        unique_fd a = make_memfd(kBufSize);
        unique_fd b = make_memfd(kBufSize);
        prod.broadcast(make_header(static_cast<uint64_t>(i)), {a.get(), b.get()});
        std::this_thread::sleep_for(std::chrono::milliseconds(20));
    }

    // OwnedFrameView's fd should still be valid — mmap and verify the marker.
    void* m = ::mmap(nullptr, kBufSize, PROT_READ, MAP_SHARED, v.fd.get(), 0);
    ASSERT_NE(m, MAP_FAILED);
    auto* bytes = static_cast<const uint8_t*>(m);
    EXPECT_EQ(bytes[0], 0xAB) << "mmap of dup'd fd should see original data";
    EXPECT_EQ(bytes[kBufSize - 1], 0xAB);
    ::munmap(m, kBufSize);

    consumer.stop();
    prod.stop();
}
