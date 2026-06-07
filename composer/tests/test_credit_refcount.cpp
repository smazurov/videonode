// Producer-side slot in-flight refcount + consumer credit echo. Host-runnable
// with real Unix sockets + memfd stand-ins for dma-bufs.

#include "src/common/unique_fd.hpp"
#include "src/ipc/dmabuf_header.hpp"
#include "src/ipc/scm_rights_producer.hpp"
#include "src/ipc/scm_socket.hpp"

#include <gtest/gtest.h>

#include <chrono>
#include <cstdio>
#include <sys/mman.h>
#include <thread>
#include <unistd.h>
#include <vector>

namespace {

using vn::base::unique_fd;

unique_fd make_fd(size_t size) {
    unique_fd fd(::memfd_create("credit_refcount_test", 0));
    if (!fd)
        return {};
    if (::ftruncate(fd.get(), static_cast<off_t>(size)) < 0)
        return {};
    return fd;
}

dmabuf_header::Header make_header(uint64_t idx, uint32_t slot, uint64_t gen) {
    dmabuf_header::Header h;
    h.slot_index = slot;
    h.width = 320;
    h.height = 240;
    h.format = "NV12";
    h.plane_pitches = {320, 320};
    h.plane_offsets = {0, 320 * 240};
    h.frame_idx = idx;
    h.generation = gen;
    return h;
}

std::string tmp_sock(const char* tag) {
    char buf[128];
    snprintf(buf, sizeof(buf), "/tmp/credit_refcount_%d_%lld_%s.sock", ::getpid(),
             static_cast<long long>(std::chrono::steady_clock::now().time_since_epoch().count()),
             tag);
    return buf;
}

template <typename F> bool wait_for(F cond, std::chrono::milliseconds timeout) {
    auto deadline = std::chrono::steady_clock::now() + timeout;
    while (std::chrono::steady_clock::now() < deadline) {
        if (cond())
            return true;
        std::this_thread::sleep_for(std::chrono::milliseconds(2));
    }
    return cond();
}

std::vector<unique_fd> adopt_fds(std::vector<int>& fds) {
    std::vector<unique_fd> out;
    out.reserve(fds.size());
    for (int f : fds)
        out.emplace_back(f);
    fds.clear();
    return out;
}

} // namespace

TEST(CreditRefcount, SlotStaysInFlightUntilCredited) {
    auto path = tmp_sock("inflight");
    scm_rights_producer::ScmRightsProducer prod;
    scm_rights_producer::InitParams p;
    p.socket_path = path;
    ASSERT_TRUE(prod.init(p));
    ASSERT_TRUE(prod.start());

    unique_fd client = scm_socket::ConnectClient(path);
    ASSERT_TRUE(client.ok());
    ASSERT_TRUE(scm_socket::SendReady(client.get()));
    ASSERT_TRUE(
        wait_for([&] { return prod.consumer_count() == 1; }, std::chrono::milliseconds(500)));

    unique_fd fd = make_fd(4096);
    ASSERT_TRUE(fd.ok());
    ASSERT_EQ(1, prod.broadcast(make_header(1, 3, 5), {fd.get(), fd.get()}));
    fd.reset();
    EXPECT_EQ(uint32_t(1), prod.inflight_for(3));

    dmabuf_header::Header rh;
    std::vector<int> rfds;
    ASSERT_TRUE(scm_socket::RecvMessage(client.get(), rh, rfds));
    auto owned = adopt_fds(rfds);
    EXPECT_EQ(uint32_t(3), rh.slot_index);
    EXPECT_EQ(uint64_t(5), rh.generation);
    EXPECT_EQ(uint32_t(1), prod.inflight_for(3));

    ASSERT_TRUE(scm_socket::SendCredit(client.get(), {.slot_index = 3, .generation = 5}));
    EXPECT_TRUE(wait_for(
        [&] {
            prod.drain_credits();
            return prod.inflight_for(3) == 0;
        },
        std::chrono::milliseconds(500)));
    EXPECT_EQ(uint32_t(0), prod.inflight_for(3));

    prod.stop();
}

TEST(CreditRefcount, PruneReleasesHeldSlotOfDeadConsumer) {
    auto path = tmp_sock("prune-release");
    scm_rights_producer::ScmRightsProducer prod;
    scm_rights_producer::InitParams p;
    p.socket_path = path;
    ASSERT_TRUE(prod.init(p));
    ASSERT_TRUE(prod.start());

    unique_fd client = scm_socket::ConnectClient(path);
    ASSERT_TRUE(client.ok());
    ASSERT_TRUE(scm_socket::SendReady(client.get()));
    ASSERT_TRUE(
        wait_for([&] { return prod.consumer_count() == 1; }, std::chrono::milliseconds(500)));

    unique_fd fd = make_fd(4096);
    ASSERT_EQ(1, prod.broadcast(make_header(1, 2, 9), {fd.get(), fd.get()}));
    fd.reset();
    EXPECT_EQ(uint32_t(1), prod.inflight_for(2));

    client.reset(); // die without crediting

    EXPECT_TRUE(wait_for(
        [&] {
            prod.prune_dead_consumers();
            return prod.consumer_count() == 0 && prod.inflight_for(2) == 0;
        },
        std::chrono::milliseconds(500)));
    EXPECT_EQ(uint32_t(0), prod.inflight_for(2));

    prod.stop();
}

TEST(CreditRefcount, BroadcastEvictionReleasesHeldSlot) {
    auto path = tmp_sock("bcast-release");
    scm_rights_producer::ScmRightsProducer prod;
    scm_rights_producer::InitParams p;
    p.socket_path = path;
    ASSERT_TRUE(prod.init(p));
    ASSERT_TRUE(prod.start());

    unique_fd client = scm_socket::ConnectClient(path);
    ASSERT_TRUE(client.ok());
    ASSERT_TRUE(scm_socket::SendReady(client.get()));
    ASSERT_TRUE(
        wait_for([&] { return prod.consumer_count() == 1; }, std::chrono::milliseconds(500)));

    unique_fd fd = make_fd(4096);
    ASSERT_EQ(1, prod.broadcast(make_header(1, 4, 1), {fd.get(), fd.get()}));
    EXPECT_EQ(uint32_t(1), prod.inflight_for(4));

    client.reset(); // die without crediting

    EXPECT_TRUE(wait_for(
        [&] {
            prod.broadcast(make_header(2, 4, 2), {fd.get(), fd.get()});
            return prod.consumer_count() == 0;
        },
        std::chrono::milliseconds(500)));
    fd.reset();
    EXPECT_EQ(uint32_t(0), prod.inflight_for(4));

    prod.stop();
}

TEST(CreditRefcount, StuckConsumerEvictedSoOthersProgress) {
    auto path = tmp_sock("stall");
    scm_rights_producer::ScmRightsProducer prod;
    scm_rights_producer::InitParams p;
    p.socket_path = path;
    p.credit_stall_frames = 5;
    ASSERT_TRUE(prod.init(p));
    ASSERT_TRUE(prod.start());

    unique_fd wedged = scm_socket::ConnectClient(path);
    unique_fd healthy = scm_socket::ConnectClient(path);
    ASSERT_TRUE(wedged.ok() && healthy.ok());
    ASSERT_TRUE(scm_socket::SendReady(wedged.get()));
    ASSERT_TRUE(scm_socket::SendReady(healthy.get()));
    ASSERT_TRUE(
        wait_for([&] { return prod.consumer_count() == 2; }, std::chrono::milliseconds(500)));

    unique_fd fd = make_fd(4096);
    ASSERT_TRUE(fd.ok());
    for (uint64_t i = 1; i <= 20 && prod.consumer_count() > 1; ++i) {
        prod.broadcast(make_header(i, 0, i), {fd.get(), fd.get()});
        dmabuf_header::Header rh;
        std::vector<int> rfds;
        if (scm_socket::RecvMessage(healthy.get(), rh, rfds)) {
            auto owned = adopt_fds(rfds);
            EXPECT_TRUE(scm_socket::SendCredit(
                healthy.get(), {.slot_index = rh.slot_index, .generation = rh.generation}));
        }
        std::this_thread::sleep_for(std::chrono::milliseconds(2));
        prod.drain_credits();
    }
    fd.reset();

    EXPECT_EQ(1, prod.consumer_count());
    prod.drain_credits();
    EXPECT_EQ(uint32_t(0), prod.inflight_for(0));

    dmabuf_header::Header rh;
    std::vector<int> rfds;
    unique_fd fd2 = make_fd(4096);
    EXPECT_EQ(1, prod.broadcast(make_header(99, 0, 99), {fd2.get(), fd2.get()}));
    fd2.reset();
    ASSERT_TRUE(scm_socket::RecvMessage(healthy.get(), rh, rfds));
    auto owned = adopt_fds(rfds);
    EXPECT_EQ(uint64_t(99), rh.frame_idx);

    prod.stop();
}

TEST(CreditRefcount, StaleGenerationCreditIgnored) {
    auto path = tmp_sock("stale");
    scm_rights_producer::ScmRightsProducer prod;
    scm_rights_producer::InitParams p;
    p.socket_path = path;
    ASSERT_TRUE(prod.init(p));
    ASSERT_TRUE(prod.start());

    unique_fd client = scm_socket::ConnectClient(path);
    ASSERT_TRUE(client.ok());
    ASSERT_TRUE(scm_socket::SendReady(client.get()));
    ASSERT_TRUE(
        wait_for([&] { return prod.consumer_count() == 1; }, std::chrono::milliseconds(500)));

    unique_fd fd = make_fd(4096);
    ASSERT_EQ(1, prod.broadcast(make_header(1, 6, 1), {fd.get(), fd.get()}));
    fd.reset();
    EXPECT_EQ(uint32_t(1), prod.inflight_for(6));

    dmabuf_header::Header rh;
    std::vector<int> rfds;
    ASSERT_TRUE(scm_socket::RecvMessage(client.get(), rh, rfds));
    auto owned = adopt_fds(rfds);

    ASSERT_TRUE(scm_socket::SendCredit(client.get(), {.slot_index = 6, .generation = 999}));
    for (int i = 0; i < 20; ++i) {
        prod.drain_credits();
        std::this_thread::sleep_for(std::chrono::milliseconds(1));
    }
    EXPECT_EQ(uint32_t(1), prod.inflight_for(6)); // stale gen rejected

    ASSERT_TRUE(scm_socket::SendCredit(client.get(), {.slot_index = 6, .generation = 1}));
    EXPECT_TRUE(wait_for(
        [&] {
            prod.drain_credits();
            return prod.inflight_for(6) == 0;
        },
        std::chrono::milliseconds(500)));

    prod.stop();
}

TEST(CreditRefcount, SentinelSlotBypassesAccounting) {
    auto path = tmp_sock("sentinel");
    scm_rights_producer::ScmRightsProducer prod;
    scm_rights_producer::InitParams p;
    p.socket_path = path;
    ASSERT_TRUE(prod.init(p));
    ASSERT_TRUE(prod.start());

    unique_fd client = scm_socket::ConnectClient(path);
    ASSERT_TRUE(client.ok());
    ASSERT_TRUE(scm_socket::SendReady(client.get()));
    ASSERT_TRUE(
        wait_for([&] { return prod.consumer_count() == 1; }, std::chrono::milliseconds(500)));

    unique_fd fd = make_fd(4096);
    ASSERT_EQ(1, prod.broadcast(make_header(1, 0xFFFFFFFFu, 0), {fd.get(), fd.get()}));
    EXPECT_EQ(uint32_t(0), prod.inflight_for(0xFFFFFFFFu));

    ASSERT_EQ(1, prod.broadcast(make_header(2, 1, 1), {fd.get(), fd.get()}));
    fd.reset();
    EXPECT_EQ(uint32_t(1), prod.inflight_for(1));

    prod.stop();
}
