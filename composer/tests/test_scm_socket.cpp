// Tests for scm_socket — uses a socketpair so we don't need filesystem
// paths or root-y permissions. SendMessage and RecvMessage round-trip the
// SCM_RIGHTS + length-prefixed JSON on the same process for verification.

#include "src/ipc/scm_socket.hpp"
#include "src/ipc/dmabuf_header.hpp"

#include <gtest/gtest.h>

#include <cstdio>
#include <cstring>
#include <fcntl.h>
#include <sys/socket.h>
#include <sys/types.h>
#include <unistd.h>

namespace {

// Create a socketpair of AF_UNIX SOCK_STREAM sockets. Returns false on error.
bool make_socketpair(int& a, int& b) {
    int sv[2];
    if (::socketpair(AF_UNIX, SOCK_STREAM | SOCK_CLOEXEC, 0, sv) < 0) {
        fprintf(stderr, "socketpair: %s\n", strerror(errno));
        return false;
    }
    a = sv[0];
    b = sv[1];
    return true;
}

// Open a temp file and write a known byte to it. Returns its fd.
int make_tempfile_with_byte(uint8_t byte) {
    char tmpl[] = "/tmp/scm_socket_test.XXXXXX";
    int fd = ::mkstemp(tmpl);
    if (fd < 0)
        return -1;
    ::unlink(tmpl); // ensure it goes away when fd closes
    ::write(fd, &byte, 1);
    ::fsync(fd);
    return fd;
}

} // namespace

TEST(ScmSocket, SingleFdRoundtrip) {
    int a, b;
    ASSERT_TRUE(make_socketpair(a, b));

    dmabuf_header::Header h;
    h.slot_index = 0;
    h.width = 1920;
    h.height = 1080;
    h.format = "NV12";
    h.plane_pitches = {1920};
    h.plane_offsets = {0};
    h.frame_idx = 1;

    int tmp = make_tempfile_with_byte(0xAB);
    EXPECT_TRUE(tmp >= 0);

    EXPECT_TRUE(scm_socket::SendMessage(a, h, {tmp}));

    dmabuf_header::Header rxHeader;
    std::vector<int> rxFds;
    bool eof = false;
    EXPECT_TRUE(scm_socket::RecvMessage(b, rxHeader, rxFds, &eof));
    EXPECT_FALSE(eof);
    EXPECT_EQ(rxHeader.slot_index, 0u);
    EXPECT_EQ(rxHeader.width, 1920u);
    EXPECT_EQ(rxHeader.height, 1080u);
    EXPECT_EQ(rxHeader.format, "NV12");
    EXPECT_EQ(rxFds.size(), 1u);
    EXPECT_TRUE(rxFds[0] >= 0);
    EXPECT_TRUE(rxFds[0] != tmp); // dup'd

    // Verify the received fd refers to the same underlying file.
    uint8_t byte = 0;
    EXPECT_TRUE(::pread(rxFds[0], &byte, 1, 0) == 1);
    EXPECT_EQ(byte, uint8_t(0xAB));

    ::close(rxFds[0]);
    ::close(tmp);
    ::close(a);
    ::close(b);
}

TEST(ScmSocket, MultiFdRoundtrip) {
    int a, b;
    ASSERT_TRUE(make_socketpair(a, b));

    dmabuf_header::Header h;
    h.slot_index = 1;
    h.width = 1280;
    h.height = 720;
    h.format = "NV12";
    h.plane_pitches = {1280, 1280};
    h.plane_offsets = {0, 0};
    h.frame_idx = 7;

    int y = make_tempfile_with_byte(0x11);
    int uv = make_tempfile_with_byte(0x22);
    EXPECT_TRUE(scm_socket::SendMessage(a, h, {y, uv}));

    dmabuf_header::Header rxHeader;
    std::vector<int> rxFds;
    EXPECT_TRUE(scm_socket::RecvMessage(b, rxHeader, rxFds, nullptr));
    EXPECT_EQ(rxFds.size(), 2u);

    uint8_t b1 = 0, b2 = 0;
    ::pread(rxFds[0], &b1, 1, 0);
    ::pread(rxFds[1], &b2, 1, 0);
    EXPECT_EQ(b1, uint8_t(0x11));
    EXPECT_EQ(b2, uint8_t(0x22));

    ::close(rxFds[0]);
    ::close(rxFds[1]);
    ::close(y);
    ::close(uv);
    ::close(a);
    ::close(b);
}

TEST(ScmSocket, EofOnCleanClose) {
    int a, b;
    ASSERT_TRUE(make_socketpair(a, b));
    ::close(a);

    dmabuf_header::Header rxHeader;
    std::vector<int> rxFds;
    bool eof = false;
    EXPECT_FALSE(scm_socket::RecvMessage(b, rxHeader, rxFds, &eof));
    EXPECT_TRUE(eof);
    ::close(b);
}
