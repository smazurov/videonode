// scm_socket — Unix-socket helpers for transporting dma-buf fds via
// SCM_RIGHTS alongside a small binary header.
//
// Wire format (post-cutover; the JSON-RPC envelope is gone):
//
//   [binary header — see ipc/dmabuf_header.hpp] + SCM_RIGHTS fds
//
// The header is self-describing (peek plane_count after the fixed
// 36-byte prefix to know the total size); no length prefix.
//
// One control channel handles one source slot. The composer's main loop
// owns the socket fd; each new arriving message replaces the slot's
// "current dma-buf fds". The composer is responsible for closing the
// previous fds before re-pointing.

#pragma once

#include "src/common/unique_fd.hpp"
#include "src/ipc/dmabuf_header.hpp"

#include <cstdint>
#include <string>
#include <vector>

namespace scm_socket {

// Bind a Unix STREAM socket at `path` and call ::listen(backlog). Returns
// the listening fd. Empty unique_fd on error with errno set. The path is
// unlinked first if it exists (we own it). Use this when the caller wants
// to manage accept() separately (e.g. defer it past init(), poll for
// multiple consumers over the socket's lifetime).
[[nodiscard]] vn::base::unique_fd BindAndListen(const std::string& path, int backlog);

// Bind a Unix STREAM socket at `path`, listen + accept ONE client, and
// return the connected client fd as a unique_fd (the internal listen fd is
// closed before we return). Empty unique_fd on error with errno set. The
// path is unlinked first if it exists (we own it).
[[nodiscard]] vn::base::unique_fd ListenAndAccept(const std::string& path);

// AcceptOne accepts a single client on `listen_fd` and returns the client
// fd. Empty unique_fd on error with errno set. For callers that hold a
// long-lived listen fd and accept multiple consumers over its lifetime.
[[nodiscard]] vn::base::unique_fd AcceptOne(int listen_fd);

// Connect to a Unix STREAM socket at `path`. Used by the testing/probe
// senders. Empty unique_fd on error with errno set.
[[nodiscard]] vn::base::unique_fd ConnectClient(const std::string& path);

// Receive one binary header + accompanying SCM_RIGHTS fds.
// On success `header_out` and `fds_out` are populated and returns true.
// On EOF (peer closed cleanly) returns false with `eof_out = true` if
// non-null. On any other failure returns false and sets errno (if a
// syscall failed) or leaves it untouched (parser failure).
//
// Caller owns the fds returned in fds_out and must close them when done.
[[nodiscard]] bool RecvMessage(int sock_fd, dmabuf_header::Header& header_out,
                               std::vector<int>& fds_out, bool* eof_out = nullptr);

// SendMessage sends a binary header + SCM_RIGHTS fds atomically.
[[nodiscard]] bool SendMessage(int sock_fd, const dmabuf_header::Header& header,
                               const std::vector<int>& fds);

} // namespace scm_socket
