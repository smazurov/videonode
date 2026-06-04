#pragma once

#include <poll.h>

namespace source {

struct CapturePollAction {
    bool drain_events = false;
    bool dequeue = false;
    bool error = false; // fd error/hangup — must recover, not ignore
};

inline CapturePollAction classify_capture_poll(short revents) {
    CapturePollAction a;
    a.dequeue = (revents & POLLIN) != 0;
    a.drain_events = (revents & POLLPRI) != 0;
    a.error = (revents & (POLLERR | POLLHUP | POLLNVAL)) != 0;
    return a;
}

} // namespace source
