/**
 * V4L2 Signal Detection - Device readiness and signal checking with event support
 */

#include "v4l2_detector.h"
#include "v4l2_utils.h"
#include <errno.h>
#include <string.h>
#include <stdlib.h>
#include <sys/ioctl.h>
#include <sys/select.h>
#include <linux/videodev2.h>
#include <libv4l2.h>
#include <fcntl.h>
#include <unistd.h>

// Suppress libv4l2 plugin messages globally
__attribute__((constructor))
static void init_v4l2_signal() {
    // Redirect libv4l2 log output to /dev/null (force override)
    setenv("LIBV4L2_LOG_FILENAME", "/dev/null", 1);
}

// Calculate FPS from pixel clock and timings
static double calculate_fps(const struct v4l2_dv_timings *timings) {
    if (timings->bt.pixelclock == 0) return 0.0;
    
    uint64_t total_width = timings->bt.width + timings->bt.hfrontporch + 
                          timings->bt.hsync + timings->bt.hbackporch;
    uint64_t total_height = timings->bt.height + timings->bt.vfrontporch + 
                           timings->bt.vsync + timings->bt.vbackporch;
    
    if (timings->bt.interlaced) {
        total_height = total_height / 2;
    }
    
    if (total_width == 0 || total_height == 0) return 0.0;
    
    double fps = (double)timings->bt.pixelclock / (double)(total_width * total_height);
    return fps;
}

// Combined device type and ready check - single device open
struct v4l2_device_status v4l2_get_device_status(const char *device_path) {
    struct v4l2_device_status status;
    status.device_type = V4L2_DEVICE_TYPE_UNKNOWN;
    status.ready = 0;

    int fd = v4l2_open(device_path, O_RDWR | O_NONBLOCK);
    if (fd < 0) {
        return status;
    }

    struct v4l2_capability cap;
    memset(&cap, 0, sizeof(cap));

    if (v4l2_ioctl(fd, VIDIOC_QUERYCAP, &cap) < 0) {
        v4l2_close(fd);
        return status;
    }

    // Check if device supports DV timings by trying the ioctl (HDMI devices)
    struct v4l2_dv_timings timings;
    memset(&timings, 0, sizeof(timings));

    int ret = v4l2_ioctl(fd, VIDIOC_G_DV_TIMINGS, &timings);
    if (ret == 0 || errno == ENOLINK || errno == ENOLCK) {
        // Device supports DV timings (HDMI)
        status.device_type = V4L2_DEVICE_TYPE_HDMI;

        // Check if signal is locked
        if (ret == 0 && timings.bt.width > 0 && timings.bt.height > 0 && timings.bt.pixelclock > 0) {
            status.ready = 1;
        }
        v4l2_close(fd);
        return status;
    }

    // Check if it's a UVC webcam
    if (strcmp((char*)cap.driver, "uvcvideo") == 0) {
        status.device_type = V4L2_DEVICE_TYPE_WEBCAM;
        status.ready = 1;
        v4l2_close(fd);
        return status;
    }

    // Everything else is unknown, but openable means ready
    status.device_type = V4L2_DEVICE_TYPE_UNKNOWN;
    status.ready = 1;
    v4l2_close(fd);
    return status;
}

// Device type detection - wrapper around v4l2_get_device_status
int v4l2_get_device_type(const char *device_path) {
    struct v4l2_device_status status = v4l2_get_device_status(device_path);
    return status.device_type;
}

// Get current DV timings (non-querying) - returns detailed status
struct v4l2_signal_status v4l2_get_dv_timings(const char *device_path) {
    struct v4l2_signal_status status;
    memset(&status, 0, sizeof(status));
    status.state = V4L2_SIGNAL_STATE_NO_DEVICE;
    
    int fd = v4l2_open(device_path, O_RDWR | O_NONBLOCK);
    if (fd < 0) {
        return status;
    }

    struct v4l2_dv_timings timings;
    memset(&timings, 0, sizeof(timings));
    
    // Use VIDIOC_G_DV_TIMINGS instead of QUERY to avoid kernel messages
    int ret = v4l2_ioctl(fd, VIDIOC_G_DV_TIMINGS, &timings);
    if (ret == 0) {
        // Check if timings are valid
        if (timings.bt.width > 0 && timings.bt.height > 0 && timings.bt.pixelclock > 0) {
            status.state = V4L2_SIGNAL_STATE_LOCKED;
            status.width = timings.bt.width;
            status.height = timings.bt.height;
            status.fps = calculate_fps(&timings);
            status.interlaced = timings.bt.interlaced;
        } else {
            status.state = V4L2_SIGNAL_STATE_NO_SIGNAL;
        }
    } else {
        // Check specific error codes
        switch (errno) {
            case ENOLINK:
                status.state = V4L2_SIGNAL_STATE_NO_LINK;
                break;
            case ENOLCK:
                status.state = V4L2_SIGNAL_STATE_UNSTABLE;
                break;
            case ERANGE:
                status.state = V4L2_SIGNAL_STATE_OUT_OF_RANGE;
                break;
            case ENOTTY:
                // Device doesn't support DV timings
                status.state = V4L2_SIGNAL_STATE_NOT_SUPPORTED;
                break;
            default:
                status.state = V4L2_SIGNAL_STATE_NO_SIGNAL;
                break;
        }
    }
    
    v4l2_close(fd);
    return status;
}

// Wait for source change event (blocking)
int v4l2_wait_for_source_change(const char *device_path, int timeout_ms) {
    int fd = v4l2_open(device_path, O_RDWR | O_NONBLOCK);
    if (fd < 0) {
        return -1;
    }

    // Subscribe to source change events
    struct v4l2_event_subscription sub;
    memset(&sub, 0, sizeof(sub));
    sub.type = V4L2_EVENT_SOURCE_CHANGE;
    
    if (v4l2_ioctl(fd, VIDIOC_SUBSCRIBE_EVENT, &sub) < 0) {
        v4l2_close(fd);
        return -2; // Events not supported
    }

    // Wait for event using select
    fd_set fds;
    struct timeval tv;
    
    FD_ZERO(&fds);
    FD_SET(fd, &fds);
    
    if (timeout_ms > 0) {
        tv.tv_sec = timeout_ms / 1000;
        tv.tv_usec = (timeout_ms % 1000) * 1000;
    }

    int ret = select(fd + 1, NULL, NULL, &fds, timeout_ms > 0 ? &tv : NULL);
    
    if (ret > 0) {
        // Event occurred, dequeue it
        struct v4l2_event ev;
        if (v4l2_ioctl(fd, VIDIOC_DQEVENT, &ev) == 0) {
            // Unsubscribe from events
            v4l2_ioctl(fd, VIDIOC_UNSUBSCRIBE_EVENT, &sub);
            v4l2_close(fd);
            
            // Return the change flags (1 = resolution change)
            return ev.u.src_change.changes;
        }
    }
    
    // Cleanup
    v4l2_ioctl(fd, VIDIOC_UNSUBSCRIBE_EVENT, &sub);
    v4l2_close(fd);
    
    return ret; // 0 for timeout, -1 for error
}

// Simplified ready check - wrapper around v4l2_get_device_status
int v4l2_device_is_ready(const char *device_path) {
    struct v4l2_device_status status = v4l2_get_device_status(device_path);
    return status.ready;
}