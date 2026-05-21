#include "control_channel.hpp"

#include "jsonrpc_msg.hpp"

#include <algorithm>
#include <cerrno>
#include <cstdio>
#include <cstring>
#include <fcntl.h>
#include <sstream>
#include <string>
#include <sys/socket.h>
#include <sys/types.h>
#include <sys/un.h>
#include <unistd.h>

namespace control_channel {

namespace {

// Encode a JSON string literal (with surrounding quotes) for safe
// injection into a params object.
std::string json_quote(const std::string& s) {
    std::string o;
    o.reserve(s.size() + 2);
    o += '"';
    for (char c : s) {
        switch (c) {
        case '"':
            o += "\\\"";
            break;
        case '\\':
            o += "\\\\";
            break;
        case '\n':
            o += "\\n";
            break;
        case '\t':
            o += "\\t";
            break;
        case '\r':
            o += "\\r";
            break;
        default:
            if (static_cast<unsigned char>(c) < 0x20) {
                char buf[8];
                std::snprintf(buf, sizeof(buf), "\\u%04x", static_cast<unsigned>(c));
                o += buf;
            } else {
                o += c;
            }
            break;
        }
    }
    o += '"';
    return o;
}

} // namespace

ControlChannel::~ControlChannel() {
    close();
}

void ControlChannel::init(std::string daemon_socket_path, std::string device_id,
                          std::string version) {
    daemon_path_ = std::move(daemon_socket_path);
    device_id_ = std::move(device_id);
    version_ = std::move(version);
    next_dial_attempt_ = std::chrono::steady_clock::now();
}

void ControlChannel::set_command_handler(CommandHandler h) {
    handler_ = std::move(h);
}

void ControlChannel::close() {
    if (fd_ >= 0) {
        ::close(fd_);
        fd_ = -1;
    }
    read_buf_.clear();
}

void ControlChannel::disconnect(const char* why) {
    if (fd_ < 0)
        return;
    fprintf(stderr, "control_channel: disconnect (%s)\n", why);
    ::close(fd_);
    fd_ = -1;
    read_buf_.clear();
    // Reset backoff schedule.
    next_dial_attempt_ = std::chrono::steady_clock::now() +
                         std::chrono::milliseconds(dial_backoff_ms_);
    dial_backoff_ms_ = std::min(dial_backoff_ms_ * 2, kMaxBackoffMs);
}

void ControlChannel::dial() {
    if (daemon_path_.empty())
        return;
    int fd = ::socket(AF_UNIX, SOCK_STREAM | SOCK_CLOEXEC, 0);
    if (fd < 0) {
        fprintf(stderr, "control_channel: socket: %s\n", std::strerror(errno));
        return;
    }
    sockaddr_un addr{};
    addr.sun_family = AF_UNIX;
    std::strncpy(addr.sun_path, daemon_path_.c_str(), sizeof(addr.sun_path) - 1);
    if (::connect(fd, reinterpret_cast<sockaddr*>(&addr), sizeof(addr)) != 0) {
        // Expected at startup before the daemon is up; only log once per
        // backoff window to keep journals quiet.
        ::close(fd);
        next_dial_attempt_ = std::chrono::steady_clock::now() +
                             std::chrono::milliseconds(dial_backoff_ms_);
        dial_backoff_ms_ = std::min(dial_backoff_ms_ * 2, kMaxBackoffMs);
        return;
    }
    // Set non-blocking for subsequent reads/writes.
    int flags = ::fcntl(fd, F_GETFL, 0);
    if (flags < 0 || ::fcntl(fd, F_SETFL, flags | O_NONBLOCK) < 0) {
        ::close(fd);
        return;
    }
    fd_ = fd;
    dial_backoff_ms_ = 500; // reset on success
    fprintf(stderr, "control_channel: connected to %s\n", daemon_path_.c_str());
    send_identify();
}

void ControlChannel::send_identify() {
    std::ostringstream params;
    params << "{";
    params << "\"device_id\":" << json_quote(device_id_);
    params << ",\"pid\":" << ::getpid();
    if (!version_.empty()) {
        params << ",\"version\":" << json_quote(version_);
    }
    params << "}";
    std::string line = jsonrpc_msg::EncodeNotification("identify", params.str());
    line += '\n';
    if (!write_line(line, /*nonblocking=*/true)) {
        disconnect("identify write failed");
    }
}

void ControlChannel::maintain() {
    if (fd_ >= 0)
        return;
    if (std::chrono::steady_clock::now() < next_dial_attempt_)
        return;
    dial();
}

int ControlChannel::add_to_poll(std::vector<pollfd>& set) const {
    if (fd_ < 0)
        return 0;
    pollfd pfd{};
    pfd.fd = fd_;
    pfd.events = POLLIN;
    set.push_back(pfd);
    return 1;
}

void ControlChannel::handle_events(short revents) {
    if (fd_ < 0)
        return;
    if (revents & (POLLERR | POLLHUP | POLLNVAL)) {
        disconnect("POLLERR/HUP/NVAL");
        return;
    }
    if (!(revents & POLLIN))
        return;
    // Drain available data into read_buf_, then dispatch complete lines.
    for (;;) {
        char chunk[4096];
        ssize_t n = ::recv(fd_, chunk, sizeof(chunk), 0);
        if (n > 0) {
            read_buf_.append(chunk, static_cast<size_t>(n));
            continue;
        }
        if (n == 0) {
            disconnect("EOF");
            return;
        }
        if (errno == EAGAIN || errno == EWOULDBLOCK)
            break;
        if (errno == EINTR)
            continue;
        disconnect("recv error");
        return;
    }
    process_lines();
}

void ControlChannel::process_lines() {
    for (;;) {
        size_t nl = read_buf_.find('\n');
        if (nl == std::string::npos)
            return;
        std::string line = read_buf_.substr(0, nl);
        read_buf_.erase(0, nl + 1);
        // Strip trailing \r if any.
        if (!line.empty() && line.back() == '\r')
            line.pop_back();
        if (line.empty())
            continue;
        dispatch_line(line);
        if (fd_ < 0)
            return; // dispatch caused disconnect
    }
}

void ControlChannel::dispatch_line(const std::string& line) {
    jsonrpc_msg::Frame frame;
    std::string err;
    if (!jsonrpc_msg::DecodeFrame(line, frame, &err)) {
        fprintf(stderr, "control_channel: bad frame: %s\n", err.c_str());
        return;
    }
    if (frame.kind == jsonrpc_msg::FrameKind::Notification) {
        // Daemon → sidecar notifications are not currently part of the
        // protocol. Log and ignore.
        fprintf(stderr, "control_channel: ignoring notification \"%s\"\n", frame.method.c_str());
        return;
    }
    if (frame.kind != jsonrpc_msg::FrameKind::Request) {
        // Response without a matching outgoing request — drop.
        return;
    }
    if (!handler_) {
        std::string resp = jsonrpc_msg::EncodeResponseError(
            -32601, "no handler registered", "", frame.id_raw);
        resp += '\n';
        write_line(resp, /*nonblocking=*/true);
        return;
    }
    IncomingRequest req;
    req.method = frame.method;
    req.params_json = frame.params_json;
    req.id_raw = frame.id_raw;
    HandlerResponse hr = handler_(req);
    std::string resp;
    if (hr.ok) {
        resp = jsonrpc_msg::EncodeResponseResult(hr.result_json, frame.id_raw);
    } else {
        resp = jsonrpc_msg::EncodeResponseError(hr.error_code, hr.error_message,
                                                hr.error_data_json, frame.id_raw);
    }
    resp += '\n';
    if (!write_line(resp, /*nonblocking=*/true)) {
        disconnect("response write failed");
    }
}

bool ControlChannel::write_line(const std::string& line, bool nonblocking) {
    if (fd_ < 0)
        return false;
    int flags = MSG_NOSIGNAL;
    if (nonblocking)
        flags |= MSG_DONTWAIT;
    size_t sent = 0;
    while (sent < line.size()) {
        ssize_t n = ::send(fd_, line.data() + sent, line.size() - sent, flags);
        if (n > 0) {
            sent += static_cast<size_t>(n);
            continue;
        }
        if (n < 0 && errno == EINTR)
            continue;
        if (n < 0 && (errno == EAGAIN || errno == EWOULDBLOCK))
            return false; // buffer full; caller decides drop vs disconnect
        return false;
    }
    return true;
}

bool ControlChannel::push_status(const std::string& params_json) {
    if (fd_ < 0)
        return false;
    std::string line = jsonrpc_msg::EncodeNotification("status", params_json);
    line += '\n';
    bool ok = write_line(line, /*nonblocking=*/true);
    if (ok) {
        ++status_pushes_;
    } else {
        ++status_drops_;
        // EAGAIN doesn't disconnect; heartbeat catches up. But any other
        // failure means the socket is broken.
        if (errno != EAGAIN && errno != EWOULDBLOCK)
            disconnect("status push fatal write error");
    }
    return ok;
}

} // namespace control_channel
