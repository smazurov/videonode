#include "src/ipc/scm_socket.hpp"

#include <array>
#include <cerrno>
#include <cstdio>
#include <cstring>
#include <fcntl.h>
#include <span>
#include <sys/socket.h>
#include <sys/types.h>
#include <sys/un.h>
#include <unistd.h>

namespace scm_socket {

namespace {

constexpr size_t kHeaderFixedPrefix = 36; // see ipc/dmabuf_header.hpp
constexpr int kMaxFds = 16;

bool set_addr(sockaddr_un& addr, const std::string& path) {
    if (path.size() + 1 > sizeof(addr.sun_path)) {
        errno = ENAMETOOLONG;
        return false;
    }
    std::memset(&addr, 0, sizeof(addr));
    addr.sun_family = AF_UNIX;
    std::memcpy(addr.sun_path, path.c_str(), path.size() + 1);
    return true;
}

bool read_full(int fd, std::span<uint8_t> buf) {
    while (!buf.empty()) {
        ssize_t r = ::read(fd, buf.data(), buf.size());
        if (r == 0) {
            errno = 0;
            return false;
        } // EOF
        if (r < 0) {
            if (errno == EINTR)
                continue;
            return false;
        }
        buf = buf.subspan(static_cast<size_t>(r));
    }
    return true;
}

// Extract SCM_RIGHTS fds from the cmsg buffer accompanying a recvmsg.
// If MSG_CTRUNC is set, logs and leaves fds_out empty.
void parse_cmsg_fds(const msghdr& m, std::vector<int>& fds_out) {
    if (m.msg_flags & MSG_CTRUNC) {
        fprintf(stderr, "scm_socket: control data truncated; some fds may have been dropped\n");
        return;
    }
    for (cmsghdr* c = CMSG_FIRSTHDR(const_cast<msghdr*>(&m)); c != nullptr;
         c = CMSG_NXTHDR(const_cast<msghdr*>(&m), c)) {
        if (c->cmsg_level != SOL_SOCKET || c->cmsg_type != SCM_RIGHTS)
            continue;
        size_t payload = c->cmsg_len - CMSG_LEN(0);
        size_t count = payload / sizeof(int);
        fds_out.resize(count);
        std::memcpy(fds_out.data(), CMSG_DATA(c), count * sizeof(int));
    }
}

void close_and_clear(std::vector<int>& fds) {
    for (int fd : fds) {
        if (fd >= 0)
            ::close(fd);
    }
    fds.clear();
}

} // namespace

int ListenAndAccept(const std::string& path) {
    ::unlink(path.c_str()); // best-effort: stale path won't bind otherwise
    int s = ::socket(AF_UNIX, SOCK_STREAM | SOCK_CLOEXEC, 0);
    if (s < 0)
        return -1;

    sockaddr_un addr{};
    if (!set_addr(addr, path)) {
        ::close(s);
        return -1;
    }
    if (::bind(s, reinterpret_cast<sockaddr*>(&addr), sizeof(addr)) < 0) {
        ::close(s);
        return -1;
    }
    if (::listen(s, 1) < 0) {
        ::close(s);
        return -1;
    }
    int c = ::accept4(s, nullptr, nullptr, SOCK_CLOEXEC);
    int saved = errno;
    ::close(s);
    errno = saved;
    return c;
}

int AcceptOne(int listen_fd) {
    return ::accept4(listen_fd, nullptr, nullptr, SOCK_CLOEXEC);
}

int ConnectClient(const std::string& path) {
    int s = ::socket(AF_UNIX, SOCK_STREAM | SOCK_CLOEXEC, 0);
    if (s < 0)
        return -1;
    sockaddr_un addr{};
    if (!set_addr(addr, path)) {
        ::close(s);
        return -1;
    }
    if (::connect(s, reinterpret_cast<sockaddr*>(&addr), sizeof(addr)) < 0) {
        int saved = errno;
        ::close(s);
        errno = saved;
        return -1;
    }
    return s;
}

bool RecvMessage(int sock_fd, dmabuf_header::Header& header_out, std::vector<int>& fds_out,
                 bool* eof_out) {
    header_out = {};
    fds_out.clear();
    if (eof_out)
        *eof_out = false;

    // First recvmsg: pull the 36-byte fixed prefix + accompanying
    // SCM_RIGHTS. The prefix's plane_count field (byte 35) tells us how
    // many trailing pitch/offset words to read in a follow-up read().
    std::array<uint8_t, kHeaderFixedPrefix> prefix{};
    iovec iov{.iov_base = prefix.data(), .iov_len = prefix.size()};
    uint8_t cmsg_buf[CMSG_SPACE(sizeof(int) * kMaxFds)];
    msghdr m{};
    m.msg_iov = &iov;
    m.msg_iovlen = 1;
    m.msg_control = cmsg_buf;
    m.msg_controllen = sizeof(cmsg_buf);

    ssize_t n = ::recvmsg(sock_fd, &m, MSG_CMSG_CLOEXEC);
    if (n == 0) {
        if (eof_out)
            *eof_out = true;
        return false;
    }
    if (n < 0)
        return false;
    if (n != static_cast<ssize_t>(prefix.size())) {
        errno = EPROTO;
        return false;
    }

    parse_cmsg_fds(m, fds_out);

    const uint8_t plane_count = prefix[35];
    if (plane_count == 0 || plane_count > dmabuf_header::kMaxPlanes) {
        close_and_clear(fds_out);
        errno = EPROTO;
        return false;
    }
    const size_t total = dmabuf_header::SerializedSize(plane_count);
    std::vector<uint8_t> bytes(total);
    std::memcpy(bytes.data(), prefix.data(), prefix.size());
    if (!read_full(sock_fd,
                   std::span<uint8_t>(bytes.data() + prefix.size(), total - prefix.size()))) {
        close_and_clear(fds_out);
        return false;
    }

    std::string err;
    if (!dmabuf_header::Decode(bytes, header_out, &err)) {
        close_and_clear(fds_out);
        fprintf(stderr, "scm_socket: dmabuf_header::Decode: %s\n", err.c_str());
        errno = EPROTO;
        return false;
    }

    if (fds_out.size() != header_out.plane_pitches.size()) {
        fprintf(stderr, "scm_socket: %zu fds vs %zu plane_pitches (mismatch)\n", fds_out.size(),
                header_out.plane_pitches.size());
        close_and_clear(fds_out);
        errno = EPROTO;
        return false;
    }
    return true;
}

bool SendMessage(int sock_fd, const dmabuf_header::Header& header, const std::vector<int>& fds) {
    if (fds.empty() || fds.size() != header.plane_pitches.size()) {
        errno = EINVAL;
        return false;
    }
    std::vector<uint8_t> body = dmabuf_header::Encode(header);
    if (body.empty()) {
        errno = EINVAL;
        return false;
    }

    // Atomic single sendmsg: header bytes in one iovec, SCM_RIGHTS in
    // ancillary. No length prefix needed — plane_count in the header
    // tells the consumer how many bytes the full message occupies.
    iovec iov;
    iov.iov_base = body.data();
    iov.iov_len = body.size();

    uint8_t cmsg_buf[CMSG_SPACE(sizeof(int) * kMaxFds)];
    msghdr m{};
    m.msg_iov = &iov;
    m.msg_iovlen = 1;
    m.msg_control = cmsg_buf;
    m.msg_controllen = CMSG_SPACE(sizeof(int) * fds.size());

    cmsghdr* c = CMSG_FIRSTHDR(&m);
    c->cmsg_level = SOL_SOCKET;
    c->cmsg_type = SCM_RIGHTS;
    c->cmsg_len = CMSG_LEN(sizeof(int) * fds.size());
    std::memcpy(CMSG_DATA(c), fds.data(), sizeof(int) * fds.size());

    ssize_t n = ::sendmsg(sock_fd, &m, MSG_NOSIGNAL);
    if (n < 0)
        return false;
    if (static_cast<size_t>(n) != body.size()) {
        errno = EIO;
        return false;
    }
    return true;
}

} // namespace scm_socket
