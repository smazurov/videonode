// Round-trip tests for the consumer→producer read-completion credit
// back-channel (scm_socket::SendCredit / RecvCredits).

#include "src/ipc/scm_socket.hpp"

#include <gtest/gtest.h>

#include <sys/socket.h>
#include <unistd.h>

#include <vector>

namespace {

struct Pair {
    int a = -1;
    int b = -1;
    Pair() {
        int fds[2];
        if (::socketpair(AF_UNIX, SOCK_STREAM, 0, fds) == 0) {
            a = fds[0];
            b = fds[1];
        }
    }
    ~Pair() {
        if (a >= 0)
            ::close(a);
        if (b >= 0)
            ::close(b);
    }
};

TEST(CreditChannel, SingleRoundTrip) {
    Pair p;
    ASSERT_GE(p.a, 0);
    EXPECT_TRUE(scm_socket::SendCredit(p.a, {.slot_index = 3, .generation = 7}));

    std::vector<scm_socket::Credit> got;
    EXPECT_EQ(scm_socket::RecvCredits(p.b, got), 1);
    ASSERT_EQ(got.size(), 1u);
    EXPECT_EQ(got[0].slot_index, 3u);
    EXPECT_EQ(got[0].generation, 7u);
}

TEST(CreditChannel, BatchedDrain) {
    Pair p;
    ASSERT_GE(p.a, 0);
    for (uint64_t i = 0; i < 5; ++i)
        EXPECT_TRUE(scm_socket::SendCredit(p.a, {.slot_index = i, .generation = i * 10}));

    std::vector<scm_socket::Credit> got;
    int n = scm_socket::RecvCredits(p.b, got);
    EXPECT_EQ(n, 5);
    ASSERT_EQ(got.size(), 5u);
    for (uint64_t i = 0; i < 5; ++i) {
        EXPECT_EQ(got[i].slot_index, i);
        EXPECT_EQ(got[i].generation, i * 10);
    }
}

TEST(CreditChannel, EmptyDrainReturnsZero) {
    Pair p;
    ASSERT_GE(p.a, 0);
    std::vector<scm_socket::Credit> got;
    EXPECT_EQ(scm_socket::RecvCredits(p.b, got), 0);
    EXPECT_TRUE(got.empty());
}

TEST(CreditChannel, PeerCloseReportsError) {
    Pair p;
    ASSERT_GE(p.a, 0);
    ::close(p.a);
    p.a = -1;
    std::vector<scm_socket::Credit> got;
    EXPECT_EQ(scm_socket::RecvCredits(p.b, got), -1);
}

} // namespace
