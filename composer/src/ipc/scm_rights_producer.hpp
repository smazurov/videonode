// scm_rights_producer — N-consumer dma-buf fd broadcaster.
//
// Counterpart to scm_rights_source. While ScmRightsSource is the consumer
// side (dial a socket, receive fds), ScmRightsProducer is the producer
// side: listen on a socket, accept N consumers, broadcast each new frame
// (header + dma-buf fds) to every connected consumer via SCM_RIGHTS.
//
// Semantics borrowed from MistServer's shm + page-drop model (see
// COMPOSITION_RESEARCH.md §9.2): single producer, multiple readers, slow
// consumers drop frames instead of back-pressuring the producer or each
// other. Each consumer is independent — disconnecting one or letting one
// fall behind doesn't affect the rest.
//
// Wire format is identical to scm_rights_source's: dmabuf_msg::Header
// (length-prefixed JSON) + SCM_RIGHTS ancillary fds. So videonode-composer's
// existing consumer code dials a Producer's socket and reads frames
// unchanged.
//
// Used by the videonode-source sidecar to publish NV12 dma-bufs to any
// number of consumers (videonode-composer instances, snapshot, AI, recording).

#pragma once

#include "src/rpc/dmabuf_msg.hpp"

#include <atomic>
#include <cstdint>
#include <mutex>
#include <string>
#include <thread>
#include <vector>

namespace scm_rights_producer {

struct InitParams {
    // Unix socket path the producer listens on. Consumers connect here.
    std::string socket_path;

    // Soft cap on concurrent consumers. New dials beyond this are
    // accept()'d and immediately closed to keep memory bounded.
    // Default 16; bump for high-fanout topologies.
    int max_consumers = 16;
};

// Per-consumer diagnostic counters. Snapshotted with stats() — caller may
// log them periodically. Exposed because dropped-frame counts are the main
// signal for "this consumer can't keep up; investigate."
struct ConsumerStats {
    int fd = -1;
    uint64_t frames_sent = 0;
    uint64_t frames_dropped = 0;   // EAGAIN/EWOULDBLOCK (consumer too slow)
    uint64_t evicted_at_frame = 0; // 0 if still connected
};

class ScmRightsProducer {
  public:
    [[nodiscard]] bool init(const InitParams& p);

    // start() spawns the accept thread. Non-blocking; returns once the
    // thread is running. Consumers may dial in any time after init().
    [[nodiscard]] bool start();

    // stop() shuts down accept + closes all consumer fds. Idempotent.
    void stop();

    ~ScmRightsProducer();
    ScmRightsProducer() = default;
    ScmRightsProducer(const ScmRightsProducer&) = delete;
    ScmRightsProducer& operator=(const ScmRightsProducer&) = delete;

    // broadcast sends one frame (header + fds) to every connected
    // consumer. Caller owns the fds; the kernel dups them per recipient,
    // so the caller may close immediately after broadcast() returns.
    //
    // For each consumer, the send is attempted with MSG_DONTWAIT. A
    // consumer whose socket buffer is full gets that one frame dropped
    // (frames_dropped++). Consumers whose socket has been closed get
    // evicted from the broadcast list.
    //
    // Returns false only if no consumers are currently connected (caller
    // may choose to skip work in that case). Per-consumer send failures
    // are not surfaced — they're recorded in stats.
    bool broadcast(const dmabuf_msg::Header& header, const std::vector<int>& fds);

    // Diagnostics. Thread-safe snapshot.
    int consumer_count() const;
    std::vector<ConsumerStats> stats() const;

    int listen_fd() const { return listen_fd_; }
    bool running() const { return running_.load(); }

  private:
    void accept_loop_();

    // Internal per-consumer entry. Mutated under consumers_mu_.
    struct Consumer {
        int fd = -1;
        uint64_t frames_sent = 0;
        uint64_t frames_dropped = 0;
    };

    InitParams params_;
    int listen_fd_ = -1;

    std::thread accept_thread_;
    std::atomic<bool> running_{false};
    std::atomic<bool> stop_requested_{false};

    mutable std::mutex consumers_mu_;
    std::vector<Consumer> consumers_;
    std::vector<ConsumerStats> evicted_;
    uint64_t frame_counter_ = 0;
};

} // namespace scm_rights_producer
