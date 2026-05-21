#include "scm_socket.hpp"

#include <arpa/inet.h>
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

bool write_full(int fd, std::span<const uint8_t> buf) {
    while (!buf.empty()) {
        // send() with MSG_NOSIGNAL so a dead peer returns EPIPE instead of
        // killing the process with SIGPIPE. Equivalent to write() for our
        // already-connected stream sockets.
        ssize_t w = ::send(fd, buf.data(), buf.size(), MSG_NOSIGNAL);
        if (w < 0) {
            if (errno == EINTR)
                continue;
            return false;
        }
        buf = buf.subspan(static_cast<size_t>(w));
    }
    return true;
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

bool RecvMessage(int sock_fd, dmabuf_msg::Header& header_out, std::vector<int>& fds_out,
                 bool* eof_out) {
    header_out = {};
    fds_out.clear();
    if (eof_out)
        *eof_out = false;

    // First recvmsg: pull 4-byte length prefix + accompanying SCM_RIGHTS.
    uint8_t prefix[4];
    iovec iov{prefix, sizeof(prefix)};
    // Space for up to 16 fds (much more than we ever expect).
    constexpr int kMaxFds = 16;
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
    if (n != static_cast<ssize_t>(sizeof(prefix))) {
        errno = EPROTO;
        return false;
    }

    uint32_t body_len = (uint32_t(prefix[0]) << 24) | (uint32_t(prefix[1]) << 16) |
                        (uint32_t(prefix[2]) << 8) | uint32_t(prefix[3]);
    if (body_len == 0 || body_len > 65536) {
        errno = EPROTO;
        return false;
    }

    // Walk the cmsg headers, collect SCM_RIGHTS fds.
    if (!(m.msg_flags & MSG_CTRUNC)) {
        for (cmsghdr* c = CMSG_FIRSTHDR(&m); c != nullptr; c = CMSG_NXTHDR(&m, c)) {
            if (c->cmsg_level == SOL_SOCKET && c->cmsg_type == SCM_RIGHTS) {
                size_t payload = c->cmsg_len - CMSG_LEN(0);
                size_t count = payload / sizeof(int);
                fds_out.resize(count);
                std::memcpy(fds_out.data(), CMSG_DATA(c), count * sizeof(int));
            }
        }
    } else {
        fprintf(stderr, "scm_socket: control data truncated; some fds may have been dropped\n");
    }

    // Now read the JSON body. It's a regular byte stream (no more cmsg).
    std::string body(body_len, '\0');
    if (!read_full(sock_fd, std::span(reinterpret_cast<uint8_t*>(body.data()), body_len))) {
        for (int fd : fds_out)
            ::close(fd);
        fds_out.clear();
        return false;
    }

    std::string err;
    if (!dmabuf_msg::DecodeFrameNotification(body, header_out, &err)) {
        for (int fd : fds_out)
            ::close(fd);
        fds_out.clear();
        fprintf(stderr, "scm_socket: DecodeFrameNotification: %s\n", err.c_str());
        errno = EPROTO;
        return false;
    }

    // Validate: number of fds matches plane_pitches length per the
    // protocol's contract. The Go sender enforces this; we double-check.
    if (fds_out.size() != header_out.plane_pitches.size()) {
        for (int fd : fds_out)
            ::close(fd);
        fds_out.clear();
        fprintf(stderr, "scm_socket: %zu fds vs %zu plane_pitches (mismatch)\n", fds_out.size(),
                header_out.plane_pitches.size());
        errno = EPROTO;
        return false;
    }
    return true;
}

bool SendMessage(int sock_fd, const dmabuf_msg::Header& header, const std::vector<int>& fds) {
    if (fds.empty() || fds.size() != header.plane_pitches.size()) {
        errno = EINVAL;
        return false;
    }
    std::string body = dmabuf_msg::EncodeFrameNotification(header);
    if (body.empty() || body.size() > 65536) {
        errno = EINVAL;
        return false;
    }
    // 4-byte big-endian length prefix.
    uint8_t prefix[4];
    uint32_t len = static_cast<uint32_t>(body.size());
    prefix[0] = static_cast<uint8_t>((len >> 24) & 0xff);
    prefix[1] = static_cast<uint8_t>((len >> 16) & 0xff);
    prefix[2] = static_cast<uint8_t>((len >> 8) & 0xff);
    prefix[3] = static_cast<uint8_t>(len & 0xff);

    // Atomic single sendmsg: prefix + body in two iovecs, SCM_RIGHTS in
    // ancillary. The previous two-write scheme corrupted the stream on
    // O_NONBLOCK consumer sockets when the body's send EAGAIN'd partway
    // (consumer got prefix+ancillary but only half the body, then next
    // frame's prefix bytes were misinterpreted as body trailer).
    iovec iov[2];
    iov[0].iov_base = prefix;
    iov[0].iov_len = sizeof(prefix);
    iov[1].iov_base = const_cast<char*>(body.data());
    iov[1].iov_len = body.size();

    uint8_t cmsg_buf[CMSG_SPACE(sizeof(int) * 16)];
    msghdr m{};
    m.msg_iov = iov;
    m.msg_iovlen = 2;
    m.msg_control = cmsg_buf;
    m.msg_controllen = CMSG_SPACE(sizeof(int) * fds.size());

    cmsghdr* c = CMSG_FIRSTHDR(&m);
    c->cmsg_level = SOL_SOCKET;
    c->cmsg_type = SCM_RIGHTS;
    c->cmsg_len = CMSG_LEN(sizeof(int) * fds.size());
    std::memcpy(CMSG_DATA(c), fds.data(), sizeof(int) * fds.size());

    ssize_t total = ssize_t(sizeof(prefix) + body.size());
    ssize_t n = ::sendmsg(sock_fd, &m, MSG_NOSIGNAL);
    if (n < 0)
        return false;
    if (n != total) {
        errno = EIO;
        return false;
    }
    return true;
}

} // namespace scm_socket
