// Tests for scm_socket — uses a socketpair so we don't need filesystem
// paths or root-y permissions. SendMessage and RecvMessage round-trip the
// SCM_RIGHTS + length-prefixed JSON on the same process for verification.

#include "../src/scm_socket.hpp"
#include "../src/dmabuf_msg.hpp"
#include "test_runner.hpp"

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

int main() {
    test_runner::start_case("single_fd_roundtrip");
    {
        int a, b;
        if (!make_socketpair(a, b)) {
            return 1;
        }

        dmabuf_msg::Header h;
        h.slot_index = 0;
        h.width = 1920;
        h.height = 1080;
        h.format = "NV12";
        h.plane_pitches = {1920};
        h.plane_offsets = {0};
        h.frame_idx = 1;

        int tmp = make_tempfile_with_byte(0xAB);
        CHECK_TRUE(tmp >= 0);

        CHECK_TRUE(scm_socket::SendMessage(a, h, {tmp}));

        dmabuf_msg::Header rxHeader;
        std::vector<int> rxFds;
        bool eof = false;
        CHECK_TRUE(scm_socket::RecvMessage(b, rxHeader, rxFds, &eof));
        CHECK_TRUE(!eof);
        CHECK_EQ(rxHeader.slot_index, 0u);
        CHECK_EQ(rxHeader.width, 1920u);
        CHECK_EQ(rxHeader.height, 1080u);
        CHECK_STR_EQ(rxHeader.format, "NV12");
        CHECK_EQ(rxFds.size(), 1u);
        CHECK_TRUE(rxFds[0] >= 0);
        CHECK_TRUE(rxFds[0] != tmp); // dup'd

        // Verify the received fd refers to the same underlying file.
        uint8_t byte = 0;
        CHECK_TRUE(::pread(rxFds[0], &byte, 1, 0) == 1);
        CHECK_EQ(byte, uint8_t(0xAB));

        ::close(rxFds[0]);
        ::close(tmp);
        ::close(a);
        ::close(b);
    }

    test_runner::start_case("multi_fd_roundtrip");
    {
        int a, b;
        if (!make_socketpair(a, b)) {
            return 1;
        }

        dmabuf_msg::Header h;
        h.slot_index = 1;
        h.width = 1280;
        h.height = 720;
        h.format = "NV12";
        h.plane_pitches = {1280, 1280};
        h.plane_offsets = {0, 0};
        h.frame_idx = 7;

        int y = make_tempfile_with_byte(0x11);
        int uv = make_tempfile_with_byte(0x22);
        CHECK_TRUE(scm_socket::SendMessage(a, h, {y, uv}));

        dmabuf_msg::Header rxHeader;
        std::vector<int> rxFds;
        CHECK_TRUE(scm_socket::RecvMessage(b, rxHeader, rxFds, nullptr));
        CHECK_EQ(rxFds.size(), 2u);

        uint8_t b1 = 0, b2 = 0;
        ::pread(rxFds[0], &b1, 1, 0);
        ::pread(rxFds[1], &b2, 1, 0);
        CHECK_EQ(b1, uint8_t(0x11));
        CHECK_EQ(b2, uint8_t(0x22));

        ::close(rxFds[0]);
        ::close(rxFds[1]);
        ::close(y);
        ::close(uv);
        ::close(a);
        ::close(b);
    }

    test_runner::start_case("eof_on_clean_close");
    {
        int a, b;
        if (!make_socketpair(a, b)) {
            return 1;
        }
        ::close(a);

        dmabuf_msg::Header rxHeader;
        std::vector<int> rxFds;
        bool eof = false;
        CHECK_TRUE(!scm_socket::RecvMessage(b, rxHeader, rxFds, &eof));
        CHECK_TRUE(eof);
        ::close(b);
    }

    return test_runner::report_and_exit_code();
}
