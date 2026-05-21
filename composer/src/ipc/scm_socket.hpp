// scm_socket — Unix-socket helpers for receiving dma-buf fds via
// SCM_RIGHTS, plus a small length-prefixed JSON header in the same recvmsg.
//
// Wire format mirrors internal/composer/dmabuf.go on the Go side:
//
//   [4 bytes big-endian: header length] [JSON header...] + SCM_RIGHTS fds
//
// One control channel handles one source slot. The composer's main loop
// owns the socket fd; each new arriving message replaces the slot's
// "current dma-buf fds". The composer is responsible for closing the
// previous fds before re-pointing.

#pragma once

#include "src/rpc/dmabuf_msg.hpp"

#include <cstdint>
#include <string>
#include <vector>

namespace scm_socket {

// Bind a Unix STREAM socket at `path`, listen for one connection. Returns
// the listening fd on success (>= 0) or -1 on error with errno set. The
// path is unlinked first if it exists (we own it).
int ListenAndAccept(const std::string& path);

// AcceptOne accepts a single client on `listen_fd` and returns the client
// fd. -1 on error with errno set. ListenAndAccept handles both already;
// this is for callers that hold the listen fd and want to accept multiple
// clients over its lifetime.
int AcceptOne(int listen_fd);

// Connect to a Unix STREAM socket at `path`. Used by the testing/probe
// senders. Returns the connected fd or -1 on error.
int ConnectClient(const std::string& path);

// Receive one length-prefixed JSON header + accompanying SCM_RIGHTS fds.
// On success `header_out` and `fds_out` are populated and returns true.
// On EOF (peer closed cleanly) returns false with `eof_out = true` if
// non-null. On any other failure returns false and sets errno (if a
// syscall failed) or leaves it untouched (parser failure).
//
// Caller owns the fds returned in fds_out and must close them when done.
bool RecvMessage(int sock_fd, dmabuf_msg::Header& header_out, std::vector<int>& fds_out,
                 bool* eof_out = nullptr);

// SendMessage sends a length-prefixed JSON header + SCM_RIGHTS fds. Used
// by tests and by any future host-side sender that wants to talk the same
// protocol from C++ (the production sender is Go).
bool SendMessage(int sock_fd, const dmabuf_msg::Header& header, const std::vector<int>& fds);

} // namespace scm_socket
