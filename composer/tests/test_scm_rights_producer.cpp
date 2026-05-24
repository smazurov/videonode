// Tests for scm_rights_producer. End-to-end with real Unix sockets +
// dup'd memfd fds standing in for dma-bufs. Host-runnable.

#include "src/common/unique_fd.hpp"
#include "src/ipc/dmabuf_header.hpp"
#include "src/ipc/scm_rights_producer.hpp"
#include "src/ipc/scm_rights_source.hpp"
#include "src/ipc/scm_socket.hpp"

#include <gtest/gtest.h>

#include <chrono>
#include <cstdio>
#include <cstring>
#include <sys/mman.h>
#include <sys/syscall.h>
#include <thread>
#include <unistd.h>
#include <utility>
#include <vector>

namespace {

using vn::base::unique_fd;

// Use memfd as a stand-in for a dma-buf fd. Any fd survives SCM_RIGHTS;
// the receiver doesn't care it's not a real dma-buf for the protocol test.
unique_fd make_fd(size_t size) {
    unique_fd fd(::memfd_create("scm_producer_test", 0));
    if (!fd)
        return {};
    if (::ftruncate(fd.get(), static_cast<off_t>(size)) < 0) {
        return {};
    }
    return fd;
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

// Adopt every fd in `fds` into unique_fd so the test cleans them up on scope
// exit. `fds` is left holding raw ints that have been released; callers
// should ignore them after this.
std::vector<unique_fd> adopt_fds(std::vector<int>& fds) {
    std::vector<unique_fd> out;
    out.reserve(fds.size());
    for (int f : fds)
        out.emplace_back(f);
    fds.clear();
    return out;
}

} // namespace

TEST(ScmRightsProducer, SingleConsumerReceivesBroadcast) {
    auto path = tmp_sock("single");
    scm_rights_producer::ScmRightsProducer prod;
    scm_rights_producer::InitParams p;
    p.socket_path = path;
    EXPECT_TRUE(prod.init(p));
    EXPECT_TRUE(prod.start());

    unique_fd client = scm_socket::ConnectClient(path);
    EXPECT_TRUE(client.ok());
    EXPECT_TRUE(scm_socket::SendReady(client.get()));
    EXPECT_TRUE(
        wait_for([&] { return prod.consumer_count() == 1; }, std::chrono::milliseconds(500)));

    unique_fd fd1 = make_fd(4096);
    unique_fd fd2 = make_fd(4096);
    EXPECT_TRUE(fd1.ok());
    EXPECT_TRUE(fd2.ok());

    EXPECT_TRUE(prod.broadcast(make_header(7), {fd1.get(), fd2.get()}));
    // Caller still owns fd1/fd2 — kernel dup'd them across the socket.
    fd1.reset();
    fd2.reset();

    dmabuf_header::Header rh;
    std::vector<int> rfds;
    bool eof = false;
    EXPECT_TRUE(scm_socket::RecvMessage(client.get(), rh, rfds, &eof));
    EXPECT_EQ(uint64_t(7), rh.frame_idx);
    EXPECT_EQ(size_t(2), rfds.size());
    EXPECT_EQ(std::string("NV12"), rh.format);

    auto owned_rfds = adopt_fds(rfds);
    prod.stop();
}

TEST(ScmRightsProducer, TwoConsumersBothReceive) {
    auto path = tmp_sock("two");
    scm_rights_producer::ScmRightsProducer prod;
    scm_rights_producer::InitParams p;
    p.socket_path = path;
    EXPECT_TRUE(prod.init(p));
    EXPECT_TRUE(prod.start());

    unique_fd c1 = scm_socket::ConnectClient(path);
    unique_fd c2 = scm_socket::ConnectClient(path);
    EXPECT_TRUE(c1.ok());
    EXPECT_TRUE(c2.ok());
    EXPECT_TRUE(scm_socket::SendReady(c1.get()));
    EXPECT_TRUE(scm_socket::SendReady(c2.get()));
    EXPECT_TRUE(
        wait_for([&] { return prod.consumer_count() == 2; }, std::chrono::milliseconds(500)));

    unique_fd fd = make_fd(4096);
    EXPECT_TRUE(prod.broadcast(make_header(42), {fd.get(), fd.get()}));
    fd.reset();

    for (unique_fd* c : {&c1, &c2}) {
        dmabuf_header::Header rh;
        std::vector<int> rfds;
        EXPECT_TRUE(scm_socket::RecvMessage(c->get(), rh, rfds));
        EXPECT_EQ(uint64_t(42), rh.frame_idx);
        EXPECT_EQ(size_t(2), rfds.size());
        auto owned = adopt_fds(rfds);
    }
    prod.stop();
}

TEST(ScmRightsProducer, BroadcastWithNoConsumersReturnsFalse) {
    auto path = tmp_sock("none");
    scm_rights_producer::ScmRightsProducer prod;
    scm_rights_producer::InitParams p;
    p.socket_path = path;
    EXPECT_TRUE(prod.init(p));
    EXPECT_TRUE(prod.start());

    unique_fd fd = make_fd(4096);
    bool ok = prod.broadcast(make_header(1), {fd.get(), fd.get()});
    EXPECT_FALSE(ok); // no consumers → nothing happened
    EXPECT_EQ(0, prod.consumer_count());

    prod.stop();
}

TEST(ScmRightsProducer, DisconnectedConsumerEvicted) {
    auto path = tmp_sock("evict");
    scm_rights_producer::ScmRightsProducer prod;
    scm_rights_producer::InitParams p;
    p.socket_path = path;
    EXPECT_TRUE(prod.init(p));
    EXPECT_TRUE(prod.start());

    unique_fd c1 = scm_socket::ConnectClient(path);
    unique_fd c2 = scm_socket::ConnectClient(path);
    EXPECT_TRUE(scm_socket::SendReady(c1.get()));
    EXPECT_TRUE(scm_socket::SendReady(c2.get()));
    EXPECT_TRUE(
        wait_for([&] { return prod.consumer_count() == 2; }, std::chrono::milliseconds(500)));

    // c1 disconnects without reading anything.
    c1.reset();

    // Pump frames; on one of these the producer's send to c1 will hit
    // EPIPE/ECONNRESET and evict.
    unique_fd fd = make_fd(4096);
    for (int i = 0; i < 5; ++i) {
        prod.broadcast(make_header(uint64_t(i + 1)), {fd.get(), fd.get()});
        std::this_thread::sleep_for(std::chrono::milliseconds(10));
    }
    fd.reset();

    EXPECT_TRUE(
        wait_for([&] { return prod.consumer_count() == 1; }, std::chrono::milliseconds(500)));

    // c2 should still receive subsequent broadcasts.
    unique_fd fd2 = make_fd(4096);
    EXPECT_TRUE(prod.broadcast(make_header(99), {fd2.get(), fd2.get()}));
    fd2.reset();

    // Drain c2 — pre-eviction broadcasts also queued for c2.
    bool saw_99 = false;
    for (int i = 0; i < 10; ++i) {
        dmabuf_header::Header rh;
        std::vector<int> rfds;
        if (!scm_socket::RecvMessage(c2.get(), rh, rfds))
            break;
        if (rh.frame_idx == 99)
            saw_99 = true;
        auto owned = adopt_fds(rfds);
        if (saw_99)
            break;
    }
    EXPECT_TRUE(saw_99);

    prod.stop();
}

// Regression: a disconnected consumer must be reaped by prune_dead_consumers()
// without the producer needing to broadcast anything. broadcast()-driven
// eviction (the DisconnectedConsumerEvicted test above) stalls during gaps
// in the frame source (e.g. V4L2 between DQBUFs, or a signal-transition
// window), so the source binary calls prune_dead_consumers() on every main-
// loop iteration regardless of broadcast cadence. If this test ever fails,
// dead consumers will pile up in production whenever the source pauses —
// see scm_rights_producer.cpp prune_dead_consumers().
TEST(ScmRightsProducer, PruneDeadConsumersEvictsWithoutBroadcast) {
    auto path = tmp_sock("prune");
    scm_rights_producer::ScmRightsProducer prod;
    scm_rights_producer::InitParams p;
    p.socket_path = path;
    EXPECT_TRUE(prod.init(p));
    EXPECT_TRUE(prod.start());

    unique_fd c1 = scm_socket::ConnectClient(path);
    unique_fd c2 = scm_socket::ConnectClient(path);
    unique_fd c3 = scm_socket::ConnectClient(path);
    EXPECT_TRUE(scm_socket::SendReady(c1.get()));
    EXPECT_TRUE(scm_socket::SendReady(c2.get()));
    EXPECT_TRUE(scm_socket::SendReady(c3.get()));
    EXPECT_TRUE(
        wait_for([&] { return prod.consumer_count() == 3; }, std::chrono::milliseconds(500)));

    // Disconnect two; do NOT call broadcast(). prune_dead_consumers() alone
    // must reap them, since real source code may go many ticks without a
    // frame to broadcast.
    c1.reset();
    c3.reset();

    // Brief settle for the kernel to propagate the peer close into POLLHUP.
    EXPECT_TRUE(
        wait_for([&] { return prod.prune_dead_consumers() == 0 && prod.consumer_count() == 1; },
                 std::chrono::milliseconds(500)));

    // c2 should still be a live consumer.
    EXPECT_EQ(1, prod.consumer_count());

    // Confirm c2 still works end-to-end after the prune.
    unique_fd fd = make_fd(4096);
    EXPECT_TRUE(prod.broadcast(make_header(42), {fd.get(), fd.get()}));
    fd.reset();

    dmabuf_header::Header rh;
    std::vector<int> rfds;
    EXPECT_TRUE(scm_socket::RecvMessage(c2.get(), rh, rfds));
    EXPECT_EQ(uint64_t(42), rh.frame_idx);
    auto owned = adopt_fds(rfds);

    prod.stop();
}

// Regression: with repeated connect→disconnect cycles, the producer's
// internal consumer list must stay bounded. Mirrors what
// videonode-sink processes do under churn (e.g. supervisor restarts).
// Before prune_dead_consumers(), this would let dead fds accumulate during
// stretches when broadcast() didn't run.
TEST(ScmRightsProducer, ChurnCyclesDoNotLeakConsumers) {
    auto path = tmp_sock("churn");
    scm_rights_producer::ScmRightsProducer prod;
    scm_rights_producer::InitParams p;
    p.socket_path = path;
    EXPECT_TRUE(prod.init(p));
    EXPECT_TRUE(prod.start());

    for (int i = 0; i < 10; ++i) {
        unique_fd c = scm_socket::ConnectClient(path);
        ASSERT_TRUE(c.ok());
        EXPECT_TRUE(scm_socket::SendReady(c.get()));
        EXPECT_TRUE(
            wait_for([&] { return prod.consumer_count() >= 1; }, std::chrono::milliseconds(200)));
        c.reset();
        // Reap without broadcasting — that's the whole point of prune.
        EXPECT_TRUE(wait_for(
            [&] {
                prod.prune_dead_consumers();
                return prod.consumer_count() == 0;
            },
            std::chrono::milliseconds(500)))
            << "cycle " << i << " left " << prod.consumer_count() << " consumers";
    }
    prod.stop();
}

TEST(ScmRightsProducer, MaxConsumersCapEnforced) {
    auto path = tmp_sock("cap");
    scm_rights_producer::ScmRightsProducer prod;
    scm_rights_producer::InitParams p;
    p.socket_path = path;
    p.max_consumers = 2;
    EXPECT_TRUE(prod.init(p));
    EXPECT_TRUE(prod.start());

    unique_fd c1 = scm_socket::ConnectClient(path);
    unique_fd c2 = scm_socket::ConnectClient(path);
    unique_fd c3 = scm_socket::ConnectClient(path);
    EXPECT_TRUE(c1.ok());
    EXPECT_TRUE(c2.ok());
    EXPECT_TRUE(c3.ok());
    EXPECT_TRUE(scm_socket::SendReady(c1.get()));
    EXPECT_TRUE(scm_socket::SendReady(c2.get()));
    EXPECT_TRUE(scm_socket::SendReady(c3.get()));

    // Give the accept loop a moment to process all three dials.
    EXPECT_TRUE(
        wait_for([&] { return prod.consumer_count() == 2; }, std::chrono::milliseconds(500)));
    // c3 was accepted, handshake completed, then closed (over cap); consumer_count stays at 2.

    prod.stop();
}

// End-to-end: ScmRightsProducer + ScmRightsSource (in dial mode) talk to
// each other. Proves the dial path used by videonode-composer + videonode-source
// works without a manually-managed socket pair.
TEST(ScmRightsProducer, SourceDialModeConsumesProducer) {
    auto path = tmp_sock("dial");
    scm_rights_producer::ScmRightsProducer prod;
    scm_rights_producer::InitParams p;
    p.socket_path = path;
    EXPECT_TRUE(prod.init(p));
    EXPECT_TRUE(prod.start());

    scm_rights_source::ScmRightsSource src;
    scm_rights_source::InitParams sp;
    sp.socket_path = path;
    sp.dial = true;
    EXPECT_TRUE(src.init(sp));
    EXPECT_TRUE(src.start());
    EXPECT_TRUE(
        wait_for([&] { return prod.consumer_count() == 1; }, std::chrono::milliseconds(500)));

    unique_fd fd = make_fd(4096);
    EXPECT_TRUE(prod.broadcast(make_header(123), {fd.get(), fd.get()}));
    fd.reset();

    EXPECT_TRUE(wait_for([&] { return src.latest_frame().frame_idx == 123; },
                         std::chrono::milliseconds(500)));
    auto fv = src.latest_frame();
    EXPECT_EQ(uint64_t(123), fv.frame_idx);
    EXPECT_TRUE(fv.fd >= 0);
    EXPECT_EQ(std::string("NV12"), fv.format);

    src.stop();
    prod.stop();
}
