#include "src/ipc/scm_socket.hpp"

#include "src/common/log_levels.hpp"

#include <array>
#include <cerrno>
#include <cstdint>
#include <cstring>
#include <fcntl.h>
#include <poll.h>
#include <span>
#include <sys/socket.h>
#include <sys/types.h>
#include <sys/un.h>
#include <unistd.h>

namespace scm_socket {

using vn::base::unique_fd;

namespace {

constexpr size_t kHeaderFixedPrefix = 36; // see ipc/dmabuf_header.hpp
constexpr int kMaxFds = 16;
// recvmsg on SOCK_STREAM may deliver SCM_RIGHTS from multiple kernel
// skbs in a single call. Each entry needs CMSG_SPACE(N*sizeof(int)).
// Size for 8 separate 2-fd entries (8 * 24 = 192 bytes) to avoid
// MSG_CTRUNC under back-pressure.
constexpr size_t kCmsgBufSize = CMSG_SPACE(sizeof(int) * kMaxFds) * 4;
constexpr uint8_t kReadyByte = 0x01;

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
// Always extracts whatever fds the kernel installed — even when
// MSG_CTRUNC is set — so the caller can close them instead of leaking.
// Multiple SCM_RIGHTS entries (from multiple kernel skbs consumed in
// one recvmsg) are concatenated into fds_out.
void parse_cmsg_fds(const msghdr& m, std::vector<int>& fds_out, bool& had_ctrunc) {
    had_ctrunc = (m.msg_flags & MSG_CTRUNC) != 0;
    int entries = 0;
    for (cmsghdr* c = CMSG_FIRSTHDR(const_cast<msghdr*>(&m)); c != nullptr;
         c = CMSG_NXTHDR(const_cast<msghdr*>(&m), c)) {
        if (c->cmsg_level != SOL_SOCKET || c->cmsg_type != SCM_RIGHTS)
            continue;
        size_t payload = c->cmsg_len - CMSG_LEN(0);
        size_t count = payload / sizeof(int);
        size_t base = fds_out.size();
        fds_out.resize(base + count);
        std::memcpy(std::span<int>(fds_out).subspan(base, count).data(), CMSG_DATA(c),
                    count * sizeof(int));
        ++entries;
    }
    if (had_ctrunc) {
        vn::log::warn("scm_socket: MSG_CTRUNC — %d SCM_RIGHTS entries, %zu fds extracted, "
                      "controllen_after=%zu",
                      entries, fds_out.size(), m.msg_controllen);
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

unique_fd BindAndListen(const std::string& path, int backlog) {
    ::unlink(path.c_str()); // best-effort: stale path won't bind otherwise
    unique_fd s(::socket(AF_UNIX, SOCK_STREAM | SOCK_CLOEXEC, 0));
    if (!s)
        return {};

    sockaddr_un addr{};
    if (!set_addr(addr, path)) {
        return {};
    }
    if (::bind(s.get(), reinterpret_cast<sockaddr*>(&addr), sizeof(addr)) < 0) {
        return {};
    }
    if (::listen(s.get(), backlog) < 0) {
        return {};
    }
    return s;
}

unique_fd ListenAndAccept(const std::string& path) {
    unique_fd s = BindAndListen(path, 1);
    if (!s)
        return {};
    unique_fd c(::accept4(s.get(), nullptr, nullptr, SOCK_CLOEXEC));
    // s closes via destructor regardless; preserve errno from accept4 on
    // failure.
    int saved = errno;
    s.reset();
    errno = saved;
    return c;
}

unique_fd AcceptOne(int listen_fd) {
    return unique_fd(::accept4(listen_fd, nullptr, nullptr, SOCK_CLOEXEC));
}

unique_fd ConnectClient(const std::string& path) {
    unique_fd s(::socket(AF_UNIX, SOCK_STREAM | SOCK_CLOEXEC, 0));
    if (!s)
        return {};
    sockaddr_un addr{};
    if (!set_addr(addr, path)) {
        return {};
    }
    if (::connect(s.get(), reinterpret_cast<sockaddr*>(&addr), sizeof(addr)) < 0) {
        // s's destructor runs on return, restore errno after that close.
        int saved = errno;
        s.reset();
        errno = saved;
        return {};
    }
    return s;
}

bool RecvMessage(int sock_fd, dmabuf_header::Header& header_out, std::vector<int>& fds_out,
                 bool* eof_out, bool* truncated_out) {
    header_out = {};
    fds_out.clear();
    if (eof_out)
        *eof_out = false;
    if (truncated_out)
        *truncated_out = false;

    // First recvmsg: pull the 36-byte fixed prefix + accompanying
    // SCM_RIGHTS. The prefix's plane_count field (byte 35) tells us how
    // many trailing pitch/offset words to read in a follow-up read().
    std::array<uint8_t, kHeaderFixedPrefix> prefix{};
    iovec iov{.iov_base = prefix.data(), .iov_len = prefix.size()};
    alignas(struct cmsghdr) uint8_t cmsg_buf[kCmsgBufSize];
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

    // Collect any SCM_RIGHTS fds the kernel installed into our fd
    // table BEFORE rejecting a short read — otherwise we'd leak the
    // installed fds (the kernel doesn't undo install on the receiver
    // side; MSG_CMSG_CLOEXEC only sets FD_CLOEXEC, it doesn't skip
    // installation).
    bool had_ctrunc = false;
    parse_cmsg_fds(m, fds_out, had_ctrunc);

    if (n != static_cast<ssize_t>(prefix.size())) {
        close_and_clear(fds_out);
        errno = EPROTO;
        return false;
    }

    const uint8_t plane_count = prefix[35];
    if (plane_count == 0 || plane_count > dmabuf_header::kMaxPlanes) {
        close_and_clear(fds_out);
        errno = EPROTO;
        return false;
    }
    const size_t total = dmabuf_header::SerializedSize(plane_count);
    std::vector<uint8_t> bytes(total);
    std::memcpy(bytes.data(), prefix.data(), prefix.size());
    if (!read_full(sock_fd, std::span<uint8_t>(bytes).subspan(prefix.size()))) {
        close_and_clear(fds_out);
        return false;
    }

    std::string err;
    if (!dmabuf_header::Decode(bytes, header_out, &err)) {
        close_and_clear(fds_out);
        vn::log::error("scm_socket: dmabuf_header::Decode: %s", err.c_str());
        errno = EPROTO;
        return false;
    }

    if (fds_out.size() != header_out.plane_pitches.size()) {
        vn::log::error("scm_socket: %zu fds vs %zu plane_pitches (mismatch)", fds_out.size(),
                       header_out.plane_pitches.size());
        close_and_clear(fds_out);
        if (truncated_out) {
            *truncated_out = true;
            return false;
        }
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

    alignas(struct cmsghdr) uint8_t cmsg_buf[CMSG_SPACE(sizeof(int) * kMaxFds)];
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

bool SendReady(int sock_fd) {
    uint8_t b = kReadyByte;
    ssize_t n;
    do {
        n = ::write(sock_fd, &b, 1);
    } while (n < 0 && errno == EINTR);
    return n == 1;
}

bool WaitForReady(int sock_fd, int timeout_ms) {
    pollfd pfd{.fd = sock_fd, .events = POLLIN, .revents = 0};
    int r;
    do {
        r = ::poll(&pfd, 1, timeout_ms);
    } while (r < 0 && errno == EINTR);
    if (r <= 0) {
        if (r == 0)
            errno = ETIMEDOUT;
        return false;
    }
    uint8_t b = 0;
    ssize_t n;
    do {
        n = ::read(sock_fd, &b, 1);
    } while (n < 0 && errno == EINTR);
    if (n != 1 || b != kReadyByte) {
        if (n >= 0)
            errno = EPROTO;
        return false;
    }
    return true;
}

} // namespace scm_socket
