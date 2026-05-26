// MSG_CTRUNC reproduction and multi-entry cmsg extraction tests.

#include "src/common/unique_fd.hpp"
#include "src/ipc/dmabuf_header.hpp"
#include "src/ipc/scm_socket.hpp"

#include <gtest/gtest.h>

#include <cstdio>
#include <cstring>
#include <sys/mman.h>
#include <sys/socket.h>
#include <unistd.h>
#include <vector>

namespace {

using vn::base::unique_fd;

unique_fd make_fd() {
    return unique_fd(::memfd_create("ctrunc_repro", 0));
}

dmabuf_header::Header make_header(uint64_t idx) {
    dmabuf_header::Header h;
    h.slot_index = 0;
    h.width = 320;
    h.height = 240;
    h.format = "NV12";
    h.plane_pitches = {320, 320};
    h.plane_offsets = {0, 320 * 240};
    h.frame_idx = idx;
    return h;
}

} // namespace

// SO_PASSCRED forces the kernel to deliver SCM_CREDENTIALS alongside
// SCM_RIGHTS — two cmsg entries per recvmsg. Verifies parse_cmsg_fds
// skips non-SCM_RIGHTS entries and still extracts the fds.
TEST(ScmCtruncRepro, PasscredDoesNotBreakFdExtraction) {
    int sv[2];
    ASSERT_EQ(0, ::socketpair(AF_UNIX, SOCK_STREAM | SOCK_CLOEXEC, 0, sv));
    unique_fd producer_fd(sv[0]);
    unique_fd consumer_fd(sv[1]);

    int on = 1;
    ASSERT_EQ(0, ::setsockopt(consumer_fd.get(), SOL_SOCKET, SO_PASSCRED, &on, sizeof(on)));

    unique_fd fd1 = make_fd();
    unique_fd fd2 = make_fd();
    ASSERT_TRUE(scm_socket::SendMessage(producer_fd.get(), make_header(1), {fd1.get(), fd2.get()}));

    dmabuf_header::Header rh;
    std::vector<int> rfds;
    bool eof = false;
    bool truncated = false;
    bool ok = scm_socket::RecvMessage(consumer_fd.get(), rh, rfds, &eof, &truncated);

    ASSERT_TRUE(ok) << "RecvMessage failed: " << strerror(errno)
                    << (truncated ? " (truncated)" : "");
    EXPECT_EQ(rfds.size(), 2u);
    EXPECT_EQ(rh.frame_idx, uint64_t(1));
    for (int fd : rfds)
        ::close(fd);
}

// Undersized cmsg buffer + SO_PASSCRED triggers real kernel MSG_CTRUNC.
// Credentials (CMSG_SPACE(12)=32 bytes) consume the 32-byte buffer,
// leaving no room for SCM_RIGHTS → MSG_CTRUNC, 0 fds delivered.
// Proves that cmsg buffer must be sized for ALL ancillary data types
// the kernel may inject, not just SCM_RIGHTS.
TEST(ScmCtruncRepro, TinyCmsgBufferCausesRealCtrunc) {
    int sv[2];
    ASSERT_EQ(0, ::socketpair(AF_UNIX, SOCK_STREAM | SOCK_CLOEXEC, 0, sv));
    unique_fd sender(sv[0]);
    unique_fd receiver(sv[1]);

    int on = 1;
    ASSERT_EQ(0, ::setsockopt(receiver.get(), SOL_SOCKET, SO_PASSCRED, &on, sizeof(on)));

    unique_fd fd1 = make_fd();
    unique_fd fd2 = make_fd();
    ASSERT_TRUE(scm_socket::SendMessage(sender.get(), make_header(42), {fd1.get(), fd2.get()}));

    // 32-byte cmsg buffer: fits credentials but not credentials + rights.
    uint8_t data_buf[128];
    iovec iov{.iov_base = data_buf, .iov_len = sizeof(data_buf)};
    alignas(struct cmsghdr) uint8_t small_cmsg[32];
    msghdr m{};
    m.msg_iov = &iov;
    m.msg_iovlen = 1;
    m.msg_control = small_cmsg;
    m.msg_controllen = sizeof(small_cmsg);

    ssize_t n = ::recvmsg(receiver.get(), &m, MSG_CMSG_CLOEXEC);
    ASSERT_GT(n, 0);
    EXPECT_TRUE(m.msg_flags & MSG_CTRUNC)
        << "expected MSG_CTRUNC with 32-byte cmsg buffer + SO_PASSCRED";

    int rights_entries = 0;
    int cred_entries = 0;
    for (cmsghdr* c = CMSG_FIRSTHDR(&m); c; c = CMSG_NXTHDR(&m, c)) {
        if (c->cmsg_level == SOL_SOCKET && c->cmsg_type == SCM_RIGHTS)
            ++rights_entries;
        else if (c->cmsg_level == SOL_SOCKET && c->cmsg_type == SCM_CREDENTIALS)
            ++cred_entries;
    }

    EXPECT_EQ(cred_entries, 1) << "credentials should fit in 32-byte buffer";
    EXPECT_EQ(rights_entries, 0) << "SCM_RIGHTS should be truncated";

    // Adequate buffer: both entries visible, no truncation.
    ASSERT_TRUE(scm_socket::SendMessage(sender.get(), make_header(43), {fd1.get(), fd2.get()}));

    alignas(struct cmsghdr) uint8_t big_cmsg[256];
    msghdr m2{};
    m2.msg_iov = &iov;
    m2.msg_iovlen = 1;
    m2.msg_control = big_cmsg;
    m2.msg_controllen = sizeof(big_cmsg);

    n = ::recvmsg(receiver.get(), &m2, MSG_CMSG_CLOEXEC);
    ASSERT_GT(n, 0);
    EXPECT_FALSE(m2.msg_flags & MSG_CTRUNC);

    int fds_found = 0;
    for (cmsghdr* c = CMSG_FIRSTHDR(&m2); c; c = CMSG_NXTHDR(&m2, c)) {
        if (c->cmsg_level == SOL_SOCKET && c->cmsg_type == SCM_RIGHTS)
            fds_found += int((c->cmsg_len - CMSG_LEN(0)) / sizeof(int));
    }
    EXPECT_EQ(fds_found, 2) << "adequate buffer must deliver both fds";
}

// RecvMessage with truncated_out: a frame with valid header but no
// SCM_RIGHTS fds (simulating MSG_CTRUNC) signals truncation and
// leaves the byte stream aligned for the next frame.
TEST(ScmCtruncRepro, TruncatedFrameRecovery) {
    int sv[2];
    ASSERT_EQ(0, ::socketpair(AF_UNIX, SOCK_STREAM | SOCK_CLOEXEC, 0, sv));
    unique_fd a(sv[0]);
    unique_fd b(sv[1]);

    // Inject a valid header via write() — no SCM_RIGHTS attached.
    dmabuf_header::Header bad_h = make_header(999);
    std::vector<uint8_t> bad_bytes = dmabuf_header::Encode(bad_h);
    ASSERT_FALSE(bad_bytes.empty());
    ssize_t w = ::write(a.get(), bad_bytes.data(), bad_bytes.size());
    ASSERT_EQ(w, static_cast<ssize_t>(bad_bytes.size()));

    dmabuf_header::Header rh;
    std::vector<int> rfds;
    bool eof = false;
    bool truncated = false;
    bool ok = scm_socket::RecvMessage(b.get(), rh, rfds, &eof, &truncated);
    EXPECT_FALSE(ok);
    EXPECT_FALSE(eof);
    EXPECT_TRUE(truncated);
    EXPECT_TRUE(rfds.empty());

    // Next frame with real SCM_RIGHTS must succeed — byte stream aligned.
    unique_fd fd1 = make_fd();
    unique_fd fd2 = make_fd();
    ASSERT_TRUE(scm_socket::SendMessage(a.get(), make_header(42), {fd1.get(), fd2.get()}));

    truncated = false;
    ok = scm_socket::RecvMessage(b.get(), rh, rfds, &eof, &truncated);
    EXPECT_TRUE(ok) << "second frame should succeed; errno=" << strerror(errno);
    EXPECT_FALSE(truncated);
    EXPECT_EQ(rh.frame_idx, uint64_t(42));
    EXPECT_EQ(rfds.size(), 2u);
    for (int fd : rfds)
        ::close(fd);
}
