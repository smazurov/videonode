#include "src/ipc/scm_socket.hpp"

#include <gtest/gtest.h>

#include <cstdint>
#include <sys/socket.h>
#include <unistd.h>
#include <vector>

namespace {

struct Pair {
    int recv_side = -1; // producer drains credits here
    int send_side = -1; // consumer emits credits here
    Pair() {
        int fds[2] = {-1, -1};
        if (::socketpair(AF_UNIX, SOCK_STREAM, 0, fds) == 0) {
            recv_side = fds[0];
            send_side = fds[1];
        }
    }
    ~Pair() {
        if (recv_side >= 0)
            ::close(recv_side);
        if (send_side >= 0)
            ::close(send_side);
    }
};

} // namespace

TEST(CreditChannel, SingleRoundTrip) {
    Pair p;
    ASSERT_GE(p.recv_side, 0);
    EXPECT_TRUE(scm_socket::SendCredit(p.send_side, {.slot_index = 7, .generation = 42}));
    std::vector<scm_socket::Credit> out;
    EXPECT_EQ(1, scm_socket::RecvCredits(p.recv_side, out));
    ASSERT_EQ(size_t(1), out.size());
    EXPECT_EQ(uint64_t(7), out[0].slot_index);
    EXPECT_EQ(uint64_t(42), out[0].generation);
}

TEST(CreditChannel, BatchedDrain) {
    Pair p;
    ASSERT_GE(p.recv_side, 0);
    for (uint64_t i = 0; i < 5; ++i)
        EXPECT_TRUE(scm_socket::SendCredit(p.send_side, {.slot_index = i, .generation = i * 10}));
    std::vector<scm_socket::Credit> out;
    EXPECT_EQ(5, scm_socket::RecvCredits(p.recv_side, out));
    ASSERT_EQ(size_t(5), out.size());
    for (uint64_t i = 0; i < 5; ++i) {
        EXPECT_EQ(i, out[i].slot_index);
        EXPECT_EQ(i * 10, out[i].generation);
    }
}

TEST(CreditChannel, EmptyDrainReturnsZero) {
    Pair p;
    ASSERT_GE(p.recv_side, 0);
    std::vector<scm_socket::Credit> out;
    EXPECT_EQ(0, scm_socket::RecvCredits(p.recv_side, out));
    EXPECT_TRUE(out.empty());
}

TEST(CreditChannel, PeerCloseReportsError) {
    Pair p;
    ASSERT_GE(p.recv_side, 0);
    ::close(p.send_side);
    p.send_side = -1;
    std::vector<scm_socket::Credit> out;
    EXPECT_EQ(-1, scm_socket::RecvCredits(p.recv_side, out));
}
