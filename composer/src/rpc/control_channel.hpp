// control_channel — videonode-source's client-side control plane.
//
// Dials the Go daemon's well-known control UDS, sends an `identify`
// notification, then services daemon Requests (set_format, get_status,
// shutdown) and emits unsolicited `status` Notifications.
//
// One blocking connect() per dial attempt; the rest of the protocol is
// non-blocking and driven from the sidecar's main poll() loop. Newline-
// delimited JSON-RPC 2.0 frames.

#pragma once

#include <chrono>
#include <cstdint>
#include <functional>
#include <poll.h>
#include <string>
#include <vector>

namespace control_channel {

// What the sidecar sees on the wire when the daemon sends a Request.
struct IncomingRequest {
    std::string method;
    std::string params_json; // raw JSON; empty if no params
    std::string id_raw;      // verbatim, echoed in the Response
};

// What the sidecar sends back. ok=true emits a `result`; ok=false emits
// an `error` with `code` + `message` (+ optional `data_json`).
struct HandlerResponse {
    bool ok = true;
    std::string result_json; // optional; if empty and ok, emits "{}"
    int64_t error_code = 0;  // when !ok
    std::string error_message;
    std::string error_data_json; // optional
};

class ControlChannel {
  public:
    using CommandHandler = std::function<HandlerResponse(const IncomingRequest&)>;

    ControlChannel() = default;
    ~ControlChannel();
    ControlChannel(const ControlChannel&) = delete;
    ControlChannel& operator=(const ControlChannel&) = delete;

    // init() captures the daemon socket path + identity. Does NOT dial
    // yet — dial happens lazily inside maintain().
    //
    // `kind` selects which client registry the daemon files this
    // connection under. "source" (the default when empty) lands in the
    // pipelinectl source map and receives set_format / status; "composer"
    // lands in the composer map and receives set_canvas / set_source /
    // set_layout / set_effects / set_source_state.
    void init(std::string daemon_socket_path, std::string device_id, std::string version,
              std::string kind = "");

    // Register the closure invoked on every incoming Request. Method
    // dispatch is the handler's responsibility.
    void set_command_handler(CommandHandler h);

    // close() drops the connection and stops trying to reconnect.
    void close();

    // Per-loop housekeeping. Dials if not connected and the backoff
    // window has elapsed. Cheap; safe to call every iteration.
    void maintain();

    // If connected, append our fd to `set` with POLLIN. Returns 1 if
    // appended, 0 if disconnected.
    int add_to_poll(std::vector<pollfd>& set) const;

    // Handle poll() revents for our fd. On read, parse newline-delimited
    // frames and dispatch. On hangup/error, disconnect.
    void handle_events(short revents);

    // Send a `status` notification. Non-blocking and fire-and-forget —
    // drops on EAGAIN (heartbeat will catch up), disconnects on any
    // other write error. Diagnostics via status_pushes() / status_drops().
    void push_status(const std::string& params_json);

    // Diagnostics for logging.
    bool connected() const { return fd_ >= 0; }
    uint64_t status_pushes() const { return status_pushes_; }
    uint64_t status_drops() const { return status_drops_; }

  private:
    void dial();
    void disconnect(const char* why);
    bool write_line(const std::string& line, bool nonblocking);
    void send_identify();
    void process_lines();
    void dispatch_line(const std::string& line);

    std::string daemon_path_;
    std::string device_id_;
    std::string version_;
    std::string kind_;
    CommandHandler handler_;

    int fd_ = -1;
    std::string read_buf_; // line-buffered

    std::chrono::steady_clock::time_point next_dial_attempt_{};
    int dial_backoff_ms_ = 500; // doubles on failure up to kMaxBackoffMs
    static constexpr int kMaxBackoffMs = 5000;

    uint64_t status_pushes_ = 0;
    uint64_t status_drops_ = 0;
};

} // namespace control_channel
