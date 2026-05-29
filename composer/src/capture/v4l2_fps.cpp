#include "src/capture/v4l2_fps.hpp"

#include <linux/videodev2.h>
#include <sys/ioctl.h>

namespace v4l2 {

uint32_t fps_from_timeperframe(uint32_t numerator, uint32_t denominator) {
    if (numerator == 0)
        return 0;
    // Rate = denominator / numerator, rounded to nearest whole fps so
    // NTSC fractions (e.g. 60000/1001 = 59.94) report as 60.
    return (denominator + numerator / 2) / numerator;
}

uint32_t query_capture_fps(const IoctlFn& io, int fd, uint32_t buf_type) {
    if (fd < 0)
        return 0;
    v4l2_streamparm parm{};
    parm.type = buf_type;
    if (io(fd, VIDIOC_G_PARM, &parm) < 0)
        return 0;
    const auto& tpf = parm.parm.capture.timeperframe;
    return fps_from_timeperframe(tpf.numerator, tpf.denominator);
}

} // namespace v4l2
