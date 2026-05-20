// Tests for scm_rights_producer. End-to-end with real Unix sockets +
// dup'd memfd fds standing in for dma-bufs. Host-runnable.

#include "../src/scm_rights_producer.hpp"
#include "../src/scm_rights_source.hpp"
#include "../src/scm_socket.hpp"
#include "../src/dmabuf_msg.hpp"
#include "test_runner.hpp"

#include <chrono>
#include <cstdio>
#include <cstring>
#include <sys/mman.h>
#include <sys/syscall.h>
#include <thread>
#include <unistd.h>
#include <vector>

namespace {

// Use memfd as a stand-in for a dma-buf fd. Any fd survives SCM_RIGHTS;
// the receiver doesn't care it's not a real dma-buf for the protocol test.
int make_fd(size_t size) {
    int fd = ::memfd_create("scm_producer_test", 0);
    if (fd < 0)
        return -1;
    if (::ftruncate(fd, static_cast<off_t>(size)) < 0) {
        ::close(fd);
        return -1;
    }
    return fd;
}

dmabuf_msg::Header make_header(uint64_t idx) {
    dmabuf_msg::Header h;
    h.slot_index = 0;
    h.width = 320;
    h.height = 240;
    h.format = "NV12";
    h.plane_pitches = {320, 320};
    h.plane_offsets = {0, 320 * 240};
    h.frame_idx = idx;
    return h;
}

// Pick a unique-ish socket path per test (PID + clock-tick).
std::string tmp_sock(const char* tag) {
    char buf[128];
    snprintf(buf, sizeof(buf), "/tmp/scm_producer_test_%d_%lld_%s.sock", ::getpid(),
             static_cast<long long>(std::chrono::steady_clock::now().time_since_epoch().count()),
             tag);
    return buf;
}

// Spin briefly waiting for `cond()` to return true, up to `timeout`.
template <typename F> bool wait_for(F cond, std::chrono::milliseconds timeout) {
    auto deadline = std::chrono::steady_clock::now() + timeout;
    while (std::chrono::steady_clock::now() < deadline) {
        if (cond())
            return true;
        std::this_thread::sleep_for(std::chrono::milliseconds(2));
    }
    return cond();
}

} // namespace

static void test_single_consumer_receives_broadcast() {
    auto path = tmp_sock("single");
    scm_rights_producer::ScmRightsProducer prod;
    scm_rights_producer::InitParams p;
    p.socket_path = path;
    CHECK_TRUE(prod.init(p));
    CHECK_TRUE(prod.start());

    int client = scm_socket::ConnectClient(path);
    CHECK_TRUE(client >= 0);
    CHECK_TRUE(
        wait_for([&] { return prod.consumer_count() == 1; }, std::chrono::milliseconds(500)));

    int fd1 = make_fd(4096);
    int fd2 = make_fd(4096);
    CHECK_TRUE(fd1 >= 0);
    CHECK_TRUE(fd2 >= 0);

    CHECK_TRUE(prod.broadcast(make_header(7), {fd1, fd2}));
    ::close(fd1); // caller closes after broadcast — kernel dup'd
    ::close(fd2);

    dmabuf_msg::Header rh;
    std::vector<int> rfds;
    bool eof = false;
    CHECK_TRUE(scm_socket::RecvMessage(client, rh, rfds, &eof));
    CHECK_EQ(uint64_t(7), rh.frame_idx);
    CHECK_EQ(size_t(2), rfds.size());
    CHECK_STR_EQ("NV12", rh.format);

    for (int fd : rfds)
        ::close(fd);
    ::close(client);
    prod.stop();
}

static void test_two_consumers_both_receive() {
    auto path = tmp_sock("two");
    scm_rights_producer::ScmRightsProducer prod;
    scm_rights_producer::InitParams p;
    p.socket_path = path;
    CHECK_TRUE(prod.init(p));
    CHECK_TRUE(prod.start());

    int c1 = scm_socket::ConnectClient(path);
    int c2 = scm_socket::ConnectClient(path);
    CHECK_TRUE(c1 >= 0);
    CHECK_TRUE(c2 >= 0);
    CHECK_TRUE(
        wait_for([&] { return prod.consumer_count() == 2; }, std::chrono::milliseconds(500)));

    int fd = make_fd(4096);
    CHECK_TRUE(prod.broadcast(make_header(42), {fd, fd}));
    ::close(fd);

    for (int c : {c1, c2}) {
        dmabuf_msg::Header rh;
        std::vector<int> rfds;
        CHECK_TRUE(scm_socket::RecvMessage(c, rh, rfds));
        CHECK_EQ(uint64_t(42), rh.frame_idx);
        CHECK_EQ(size_t(2), rfds.size());
        for (int f : rfds)
            ::close(f);
        ::close(c);
    }
    prod.stop();
}

static void test_broadcast_with_no_consumers_returns_false() {
    auto path = tmp_sock("none");
    scm_rights_producer::ScmRightsProducer prod;
    scm_rights_producer::InitParams p;
    p.socket_path = path;
    CHECK_TRUE(prod.init(p));
    CHECK_TRUE(prod.start());

    int fd = make_fd(4096);
    bool ok = prod.broadcast(make_header(1), {fd, fd});
    ::close(fd);
    CHECK_TRUE(!ok); // no consumers → nothing happened
    CHECK_EQ(0, prod.consumer_count());

    prod.stop();
}

static void test_disconnected_consumer_evicted() {
    auto path = tmp_sock("evict");
    scm_rights_producer::ScmRightsProducer prod;
    scm_rights_producer::InitParams p;
    p.socket_path = path;
    CHECK_TRUE(prod.init(p));
    CHECK_TRUE(prod.start());

    int c1 = scm_socket::ConnectClient(path);
    int c2 = scm_socket::ConnectClient(path);
    CHECK_TRUE(
        wait_for([&] { return prod.consumer_count() == 2; }, std::chrono::milliseconds(500)));

    // c1 disconnects without reading anything.
    ::close(c1);

    // Pump frames; on one of these the producer's send to c1 will hit
    // EPIPE/ECONNRESET and evict.
    int fd = make_fd(4096);
    for (int i = 0; i < 5; ++i) {
        prod.broadcast(make_header(uint64_t(i + 1)), {fd, fd});
        std::this_thread::sleep_for(std::chrono::milliseconds(10));
    }
    ::close(fd);

    CHECK_TRUE(
        wait_for([&] { return prod.consumer_count() == 1; }, std::chrono::milliseconds(500)));

    // c2 should still receive subsequent broadcasts.
    int fd2 = make_fd(4096);
    CHECK_TRUE(prod.broadcast(make_header(99), {fd2, fd2}));
    ::close(fd2);

    // Drain c2 — pre-eviction broadcasts also queued for c2.
    bool saw_99 = false;
    for (int i = 0; i < 10; ++i) {
        dmabuf_msg::Header rh;
        std::vector<int> rfds;
        if (!scm_socket::RecvMessage(c2, rh, rfds))
            break;
        if (rh.frame_idx == 99)
            saw_99 = true;
        for (int f : rfds)
            ::close(f);
        if (saw_99)
            break;
    }
    CHECK_TRUE(saw_99);

    ::close(c2);
    prod.stop();
}

static void test_max_consumers_cap_enforced() {
    auto path = tmp_sock("cap");
    scm_rights_producer::ScmRightsProducer prod;
    scm_rights_producer::InitParams p;
    p.socket_path = path;
    p.max_consumers = 2;
    CHECK_TRUE(prod.init(p));
    CHECK_TRUE(prod.start());

    int c1 = scm_socket::ConnectClient(path);
    int c2 = scm_socket::ConnectClient(path);
    int c3 = scm_socket::ConnectClient(path);
    CHECK_TRUE(c1 >= 0);
    CHECK_TRUE(c2 >= 0);
    CHECK_TRUE(c3 >= 0);

    // Give the accept loop a moment to process all three dials.
    CHECK_TRUE(
        wait_for([&] { return prod.consumer_count() == 2; }, std::chrono::milliseconds(500)));
    // c3 was accepted then immediately closed; consumer_count stays at 2.

    ::close(c1);
    ::close(c2);
    ::close(c3);
    prod.stop();
}

// End-to-end: ScmRightsProducer + ScmRightsSource (in dial mode) talk to
// each other. Proves the dial path used by composer-spike + videonode-source
// works without a manually-managed socket pair.
static void test_source_dial_mode_consumes_producer() {
    auto path = tmp_sock("dial");
    scm_rights_producer::ScmRightsProducer prod;
    scm_rights_producer::InitParams p;
    p.socket_path = path;
    CHECK_TRUE(prod.init(p));
    CHECK_TRUE(prod.start());

    scm_rights_source::ScmRightsSource src;
    scm_rights_source::InitParams sp;
    sp.socket_path = path;
    sp.dial = true;
    CHECK_TRUE(src.init(sp));
    CHECK_TRUE(src.start());
    CHECK_TRUE(
        wait_for([&] { return prod.consumer_count() == 1; }, std::chrono::milliseconds(500)));

    int fd = make_fd(4096);
    CHECK_TRUE(prod.broadcast(make_header(123), {fd, fd}));
    ::close(fd);

    CHECK_TRUE(wait_for([&] { return src.latest_frame().frame_idx == 123; },
                        std::chrono::milliseconds(500)));
    auto fv = src.latest_frame();
    CHECK_EQ(uint64_t(123), fv.frame_idx);
    CHECK_TRUE(fv.fd >= 0);
    CHECK_STR_EQ("NV12", fv.format);

    src.stop();
    prod.stop();
}

int main() {
    test_runner::start_case("single_consumer_receives_broadcast");
    test_single_consumer_receives_broadcast();
    test_runner::start_case("two_consumers_both_receive");
    test_two_consumers_both_receive();
    test_runner::start_case("broadcast_with_no_consumers_returns_false");
    test_broadcast_with_no_consumers_returns_false();
    test_runner::start_case("disconnected_consumer_evicted");
    test_disconnected_consumer_evicted();
    test_runner::start_case("max_consumers_cap_enforced");
    test_max_consumers_cap_enforced();
    test_runner::start_case("source_dial_mode_consumes_producer");
    test_source_dial_mode_consumes_producer();
    return test_runner::report_and_exit_code();
}
