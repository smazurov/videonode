// End-to-end test for ScmRightsSource: bind + accept + receive one
// message + verify the latest_frame() snapshot reflects it. We act as
// the "daemon" by directly using scm_socket::SendMessage on the client
// side; the receiver under test is ScmRightsSource on the server side.

#include "src/common/unique_fd.hpp"
#include "src/ipc/dmabuf_header.hpp"
#include "src/ipc/scm_rights_source.hpp"
#include "src/ipc/scm_socket.hpp"

#include <gtest/gtest.h>

#include <chrono>
#include <cstdio>
#include <cstring>
#include <fcntl.h>
#include <thread>
#include <unistd.h>

namespace {

using vn::base::unique_fd;

std::string make_tempdir_socket(const char* prefix) {
    char tmpl[] = "/tmp/scm_src_test.XXXXXX";
    int fd = ::mkstemp(tmpl);
    ::close(fd);
    ::unlink(tmpl);
    return std::string(tmpl) + "-" + prefix + ".sock";
}

unique_fd make_tempfile(uint8_t byte) {
    char tmpl[] = "/tmp/scm_src_payload.XXXXXX";
    unique_fd fd(::mkstemp(tmpl));
    ::unlink(tmpl);
    ::write(fd.get(), &byte, 1);
    ::fsync(fd.get());
    return fd;
}

} // namespace

TEST(ScmRightsSource, EndToEndSinglePlane) {
    std::string sock = make_tempdir_socket("a");

    scm_rights_source::ScmRightsSource src;
    scm_rights_source::InitParams p;
    p.socket_path = sock;
    EXPECT_TRUE(src.init(p));

    // Spawn a "daemon" thread that connects and sends one message.
    std::thread daemon([sock]() {
        // Give the server a moment to call accept (start() runs after init()).
        std::this_thread::sleep_for(std::chrono::milliseconds(100));
        unique_fd c = scm_socket::ConnectClient(sock);
        if (!c) {
            fprintf(stderr, "daemon connect: %s\n", strerror(errno));
            return;
        }

        dmabuf_header::Header h;
        h.slot_index = 0;
        h.width = 1920;
        h.height = 1080;
        h.format = "NV12";
        h.plane_pitches = {1920};
        h.plane_offsets = {0};
        h.frame_idx = 42;

        unique_fd payload = make_tempfile(0xAB);
        EXPECT_TRUE(scm_socket::SendMessage(c.get(), h, {payload.get()}));

        // Hold the connection open briefly so the receiver has time
        // to read; then close (unique_fd destructors do that on return).
        // The receiver's thread will see EOF and exit.
        std::this_thread::sleep_for(std::chrono::milliseconds(200));
    });

    EXPECT_TRUE(src.start());

    // Wait for the receiver thread to consume the message and update latest_.
    auto deadline = std::chrono::steady_clock::now() + std::chrono::seconds(2);
    scm_rights_source::FrameView v;
    while (std::chrono::steady_clock::now() < deadline) {
        v = src.latest_frame();
        if (v.frame_idx > 0)
            break;
        std::this_thread::sleep_for(std::chrono::milliseconds(10));
    }
    EXPECT_EQ(v.frame_idx, uint64_t(42));
    EXPECT_EQ(v.width, 1920);
    EXPECT_EQ(v.height, 1080);
    EXPECT_EQ(v.plane0_pitch, 1920u);
    EXPECT_EQ(v.plane0_offset, 0u);
    EXPECT_TRUE(v.fd >= 0);

    // Sanity: the received fd is a valid dup of the daemon's tempfile.
    uint8_t b = 0;
    EXPECT_TRUE(::pread(v.fd, &b, 1, 0) == 1);
    EXPECT_EQ(b, uint8_t(0xAB));

    daemon.join();
    src.stop();
}

TEST(ScmRightsSource, InitRejectsEmptyPath) {
    scm_rights_source::ScmRightsSource src;
    scm_rights_source::InitParams p;
    // socket_path left blank
    EXPECT_FALSE(src.init(p));
}

TEST(ScmRightsSource, StopUnblocksPendingAccept) {
    // start() blocks on accept; stop() should unblock by closing the
    // listen socket and the worker bail out.
    std::string sock = make_tempdir_socket("b");
    scm_rights_source::ScmRightsSource src;
    scm_rights_source::InitParams p;
    p.socket_path = sock;
    EXPECT_TRUE(src.init(p));

    // start() in a thread because it blocks for ~30s waiting for
    // accept.
    bool start_returned = false;
    bool start_result = true;
    std::thread st([&src, &start_returned, &start_result]() {
        start_result = src.start(); // expected false because we'll stop it
        start_returned = true;
    });
    std::this_thread::sleep_for(std::chrono::milliseconds(100));
    src.stop();
    st.join();
    EXPECT_TRUE(start_returned);
    EXPECT_FALSE(start_result);
}
