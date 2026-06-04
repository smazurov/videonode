#include "src/common/unique_fd.hpp"
#include "src/ipc/dmabuf_header.hpp"
#include "src/ipc/scm_socket.hpp"

#include <gtest/gtest.h>

#include <cstdio>
#include <cstring>
#include <fcntl.h>
#include <sys/socket.h>
#include <sys/types.h>
#include <unistd.h>

namespace {

using vn::base::unique_fd;

bool make_socketpair(unique_fd& a, unique_fd& b) {
    int sv[2];
    if (::socketpair(AF_UNIX, SOCK_STREAM | SOCK_CLOEXEC, 0, sv) < 0) {
        fprintf(stderr, "socketpair: %s\n", strerror(errno));
        return false;
    }
    a.reset(sv[0]);
    b.reset(sv[1]);
    return true;
}

unique_fd make_tempfile_with_byte(uint8_t byte) {
    char tmpl[] = "/tmp/scm_socket_test.XXXXXX";
    unique_fd fd(::mkstemp(tmpl));
    if (!fd)
        return {};
    ::unlink(tmpl); // ensure it goes away when fd closes
    ::write(fd.get(), &byte, 1);
    ::fsync(fd.get());
    return fd;
}

} // namespace

TEST(ScmSocket, SingleFdRoundtrip) {
    unique_fd a, b;
    ASSERT_TRUE(make_socketpair(a, b));

    dmabuf_header::Header h;
    h.slot_index = 0;
    h.width = 1920;
    h.height = 1080;
    h.format = "NV12";
    h.plane_pitches = {1920};
    h.plane_offsets = {0};
    h.frame_idx = 1;

    unique_fd tmp = make_tempfile_with_byte(0xAB);
    EXPECT_TRUE(tmp.ok());

    EXPECT_TRUE(scm_socket::SendMessage(a.get(), h, {tmp.get()}));

    dmabuf_header::Header rxHeader;
    std::vector<int> rxFds;
    bool eof = false;
    EXPECT_TRUE(scm_socket::RecvMessage(b.get(), rxHeader, rxFds, &eof));
    EXPECT_FALSE(eof);
    EXPECT_EQ(rxHeader.slot_index, 0u);
    EXPECT_EQ(rxHeader.width, 1920u);
    EXPECT_EQ(rxHeader.height, 1080u);
    EXPECT_EQ(rxHeader.format, "NV12");
    EXPECT_EQ(rxFds.size(), 1u);
    ASSERT_EQ(rxFds.size(), 1u);
    unique_fd rx0(rxFds[0]);
    EXPECT_TRUE(rx0.ok());
    EXPECT_NE(rx0.get(), tmp.get()); // dup'd

    uint8_t byte = 0;
    EXPECT_TRUE(::pread(rx0.get(), &byte, 1, 0) == 1);
    EXPECT_EQ(byte, uint8_t(0xAB));
}

TEST(ScmSocket, MultiFdRoundtrip) {
    unique_fd a, b;
    ASSERT_TRUE(make_socketpair(a, b));

    dmabuf_header::Header h;
    h.slot_index = 1;
    h.width = 1280;
    h.height = 720;
    h.format = "NV12";
    h.plane_pitches = {1280, 1280};
    h.plane_offsets = {0, 0};
    h.frame_idx = 7;

    unique_fd y = make_tempfile_with_byte(0x11);
    unique_fd uv = make_tempfile_with_byte(0x22);
    EXPECT_TRUE(scm_socket::SendMessage(a.get(), h, {y.get(), uv.get()}));

    dmabuf_header::Header rxHeader;
    std::vector<int> rxFds;
    EXPECT_TRUE(scm_socket::RecvMessage(b.get(), rxHeader, rxFds, nullptr));
    ASSERT_EQ(rxFds.size(), 2u);
    unique_fd rx0(rxFds[0]);
    unique_fd rx1(rxFds[1]);

    uint8_t b1 = 0, b2 = 0;
    ::pread(rx0.get(), &b1, 1, 0);
    ::pread(rx1.get(), &b2, 1, 0);
    EXPECT_EQ(b1, uint8_t(0x11));
    EXPECT_EQ(b2, uint8_t(0x22));
}

TEST(ScmSocket, ReadyHandshakeRoundtrip) {
    unique_fd a, b;
    ASSERT_TRUE(make_socketpair(a, b));
    EXPECT_TRUE(scm_socket::SendReady(a.get()));
    EXPECT_TRUE(scm_socket::WaitForReady(b.get(), 1000));
}

TEST(ScmSocket, WaitForReadyTimesOut) {
    unique_fd a, b;
    ASSERT_TRUE(make_socketpair(a, b));
    EXPECT_FALSE(scm_socket::WaitForReady(b.get(), 50));
    EXPECT_EQ(errno, ETIMEDOUT);
}

TEST(ScmSocket, EofOnCleanClose) {
    unique_fd a, b;
    ASSERT_TRUE(make_socketpair(a, b));
    a.reset();

    dmabuf_header::Header rxHeader;
    std::vector<int> rxFds;
    bool eof = false;
    EXPECT_FALSE(scm_socket::RecvMessage(b.get(), rxHeader, rxFds, &eof));
    EXPECT_TRUE(eof);
}
