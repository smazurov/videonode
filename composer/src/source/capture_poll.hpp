// capture_poll — pure poll(2) revents → action decision for the source
// capture fd. Header-only so both the orchestrator loop and its unit test
// share one definition.

#pragma once

#include <poll.h>

namespace source {

// CapturePollAction says what the capture loop should do with one poll()
// result on the V4L2 capture fd.
struct CapturePollAction {
    bool drain_events = false; // priority events pending (POLLPRI)
    bool dequeue = false;      // a buffer is ready (POLLIN)
    bool error = false;        // fd error/hangup — must recover, not ignore
};

inline CapturePollAction classify_capture_poll(short revents) {
    CapturePollAction a;
    a.dequeue = (revents & POLLIN) != 0;
    a.drain_events = (revents & POLLPRI) != 0;
    a.error = (revents & (POLLERR | POLLHUP | POLLNVAL)) != 0;
    return a;
}

} // namespace source
