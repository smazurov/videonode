/**
 * V4L2 Camera Detector - Format Functions
 *
 * This project is inspired by and derived from the V4L2 plugin code found in OBS Studio.
 * It is licensed under All Rights Reserved.
 */

#include "v4l2_detector.h"
#include "v4l2_utils.h"

#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <fcntl.h>
#include <unistd.h>
#include <errno.h>
#include <sys/ioctl.h>
#include <linux/videodev2.h>
#include <libv4l2.h>

// Common resolutions for continuous/stepwise formats
const struct
{
    uint32_t width;
    uint32_t height;
} common_resolutions[] = {
    // 4:3
    {160, 120},
    {320, 240},
    {480, 320},
    {640, 480},
    {800, 600},
    {1024, 768},
    {1280, 960},
    {1440, 1050},
    {1440, 1080},
    {1600, 1200},
    // 16:9
    {640, 360},
    {960, 540},
    {1280, 720},
    {1600, 900},
    {1920, 1080},
    {1920, 1200},
    {2560, 1440},
    {3840, 2160},
    // 21:9
    {2560, 1080},
    {3440, 1440},
    {5120, 2160},
    // tv
    {432, 520},
    {480, 320},
    {480, 530},
    {486, 440},
    {576, 310},
    {576, 520},
    {576, 570},
    {720, 576},
    {1024, 576}};
const size_t num_common_resolutions = sizeof(common_resolutions) / sizeof(common_resolutions[0]);

// Common framerates for continuous/stepwise intervals
const struct
{
    uint32_t numerator;
    uint32_t denominator;
} common_framerates[] = {
    {1, 60}, {1, 50}, {1, 30}, {1, 25}, {1, 20}, {1, 15}, {1, 10}, {1, 5}};
const size_t num_common_framerates = sizeof(common_framerates) / sizeof(common_framerates[0]);

// Implementations of format functions

int v4l2_get_formats(const char *device_path, struct v4l2_format_info **formats, size_t *count)
{
    int fd;
    int fmt_count = 0;
    struct v4l2_format_info *fmt_list = NULL;

    fd = v4l2_open(device_path, O_RDWR | O_NONBLOCK);
    if (fd == -1)
    {
        LOG_ERROR("Unable to open %s", device_path);
        return -errno;
    }

    // First pass: count formats
    struct v4l2_fmtdesc fmt;
    fmt.type = V4L2_BUF_TYPE_VIDEO_CAPTURE;
    fmt.index = 0;

    while (v4l2_ioctl(fd, VIDIOC_ENUM_FMT, &fmt) == 0)
    {
        fmt_count++;
        fmt.index++;
    }

    if (fmt_count == 0)
    {
        *formats = NULL;
        *count = 0;
        v4l2_close(fd);
        return 0;
    }

    // Allocate format list
    fmt_list = calloc(fmt_count, sizeof(struct v4l2_format_info));
    if (!fmt_list)
    {
        LOG_ERROR("Failed to allocate memory for format list");
        v4l2_close(fd);
        return -ENOMEM;
    }

    // Second pass: fill format list
    fmt.type = V4L2_BUF_TYPE_VIDEO_CAPTURE;
    fmt.index = 0;

    int index = 0;
    while (v4l2_ioctl(fd, VIDIOC_ENUM_FMT, &fmt) == 0 && index < fmt_count)
    {
        char format_description[64];

        // Format the description string
        if (fmt.flags & V4L2_FMT_FLAG_EMULATED)
        {
            snprintf(format_description, sizeof(format_description),
                     "%s (Emulated)", fmt.description);
        }
        else
        {
            snprintf(format_description, sizeof(format_description),
                     "%s", fmt.description);
        }

        fmt_list[index].pixel_format = fmt.pixelformat;
        fmt_list[index].format_name = strdup(format_description);
        fmt_list[index].emulated = (fmt.flags & V4L2_FMT_FLAG_EMULATED) != 0;

        char fourcc[5];
        v4l2_fourcc_to_str(fmt.pixelformat, fourcc);
        LOG_INFO("Format: %s (%s) %s", format_description, fourcc,
                 fmt_list[index].emulated ? "Emulated" : "");

        fmt.index++;
        index++;
    }

    v4l2_close(fd);

    *formats = fmt_list;
    *count = fmt_count;

    return 0;
}

int v4l2_get_resolutions(const char *device_path, uint32_t pixel_format,
                         struct v4l2_resolution **resolutions, size_t *count)
{
    int fd;
    int res_count = 0;
    struct v4l2_resolution *res_list = NULL;

    fd = v4l2_open(device_path, O_RDWR | O_NONBLOCK);
    if (fd == -1)
    {
        LOG_ERROR("Unable to open %s", device_path);
        return -errno;
    }

    struct v4l2_frmsizeenum frmsize;
    frmsize.pixel_format = pixel_format;
    frmsize.index = 0;

    // First get the framezise type
    if (v4l2_ioctl(fd, VIDIOC_ENUM_FRAMESIZES, &frmsize) == -1)
    {
        LOG_ERROR("Failed to get frame size type");
        v4l2_close(fd);
        return -errno;
    }

    // Handle different frame size types
    if (frmsize.type == V4L2_FRMSIZE_TYPE_DISCRETE)
    {
        // Count discrete frame sizes
        while (v4l2_ioctl(fd, VIDIOC_ENUM_FRAMESIZES, &frmsize) == 0)
        {
            res_count++;
            frmsize.index++;
        }
    }
    else
    {
        // For continuous/stepwise, we'll use our hardcoded set of resolutions
        // Start with common resolutions and check if they're in the range

        // Reset framesizes to get min/max values
        frmsize.index = 0;
        if (v4l2_ioctl(fd, VIDIOC_ENUM_FRAMESIZES, &frmsize) == -1)
        {
            LOG_ERROR("Failed to get framezise range");
            v4l2_close(fd);
            return -errno;
        }

        // Count resolutions in the range
        if (frmsize.type == V4L2_FRMSIZE_TYPE_CONTINUOUS ||
            frmsize.type == V4L2_FRMSIZE_TYPE_STEPWISE)
        {

            uint32_t min_width = frmsize.stepwise.min_width;
            uint32_t max_width = frmsize.stepwise.max_width;
            uint32_t min_height = frmsize.stepwise.min_height;
            uint32_t max_height = frmsize.stepwise.max_height;

            for (size_t i = 0; i < num_common_resolutions; i++)
            {
                if (common_resolutions[i].width >= min_width &&
                    common_resolutions[i].width <= max_width &&
                    common_resolutions[i].height >= min_height &&
                    common_resolutions[i].height <= max_height)
                {
                    res_count++;
                }
            }
        }
    }

    if (res_count == 0)
    {
        *resolutions = NULL;
        *count = 0;
        v4l2_close(fd);
        return 0;
    }

    // Allocate resolution list
    res_list = calloc(res_count, sizeof(struct v4l2_resolution));
    if (!res_list)
    {
        LOG_ERROR("Failed to allocate memory for resolution list");
        v4l2_close(fd);
        return -ENOMEM;
    }

    // Fill resolution list
    if (frmsize.type == V4L2_FRMSIZE_TYPE_DISCRETE)
    {
        // Reset enumeration
        frmsize.index = 0;

        int index = 0;
        while (v4l2_ioctl(fd, VIDIOC_ENUM_FRAMESIZES, &frmsize) == 0 && index < res_count)
        {
            res_list[index].width = frmsize.discrete.width;
            res_list[index].height = frmsize.discrete.height;

            // LOG_INFO("Resolution: %dx%d", res_list[index].width, res_list[index].height);

            frmsize.index++;
            index++;
        }
    }
    else
    {
        // For continuous/stepwise, use common resolutions that fit in the range

        // Reset framesizes to get min/max values
        frmsize.index = 0;
        if (v4l2_ioctl(fd, VIDIOC_ENUM_FRAMESIZES, &frmsize) == -1)
        {
            LOG_ERROR("Failed to get framezise range");
            free(res_list);
            v4l2_close(fd);
            return -errno;
        }

        if (frmsize.type == V4L2_FRMSIZE_TYPE_CONTINUOUS ||
            frmsize.type == V4L2_FRMSIZE_TYPE_STEPWISE)
        {

            uint32_t min_width = frmsize.stepwise.min_width;
            uint32_t max_width = frmsize.stepwise.max_width;
            uint32_t min_height = frmsize.stepwise.min_height;
            uint32_t max_height = frmsize.stepwise.max_height;

            int index = 0;
            for (size_t i = 0; i < num_common_resolutions && index < res_count; i++)
            {
                if (common_resolutions[i].width >= min_width &&
                    common_resolutions[i].width <= max_width &&
                    common_resolutions[i].height >= min_height &&
                    common_resolutions[i].height <= max_height)
                {

                    res_list[index].width = common_resolutions[i].width;
                    res_list[index].height = common_resolutions[i].height;

                    LOG_INFO("Resolution: %dx%d", res_list[index].width, res_list[index].height);

                    index++;
                }
            }
        }
    }

    v4l2_close(fd);

    *resolutions = res_list;
    *count = res_count;

    return 0;
}

int v4l2_get_framerates(const char *device_path, uint32_t pixel_format,
                        int width, int height,
                        struct v4l2_framerate **framerates, size_t *count)
{
    int fd;
    int rate_count = 0;
    struct v4l2_framerate *rate_list = NULL;

    fd = v4l2_open(device_path, O_RDWR | O_NONBLOCK);
    if (fd == -1)
    {
        LOG_ERROR("Unable to open %s", device_path);
        return -errno;
    }

    struct v4l2_frmivalenum frmival;
    frmival.pixel_format = pixel_format;
    frmival.width = width;
    frmival.height = height;
    frmival.index = 0;

    // First check the framerate type
    if (v4l2_ioctl(fd, VIDIOC_ENUM_FRAMEINTERVALS, &frmival) == -1)
    {
        LOG_ERROR("Failed to get frame interval type");
        v4l2_close(fd);
        return -errno;
    }

    // Handle different interval types
    if (frmival.type == V4L2_FRMIVAL_TYPE_DISCRETE)
    {
        // Count discrete frame intervals
        while (v4l2_ioctl(fd, VIDIOC_ENUM_FRAMEINTERVALS, &frmival) == 0)
        {
            rate_count++;
            frmival.index++;
        }
    }
    else
    {
        // For continuous/stepwise, we'll use our hardcoded set of framerates
        // Common framerates

        // Reset frameintervals to get min/max values
        frmival.index = 0;
        if (v4l2_ioctl(fd, VIDIOC_ENUM_FRAMEINTERVALS, &frmival) == -1)
        {
            LOG_ERROR("Failed to get frameival range");
            v4l2_close(fd);
            return -errno;
        }

        if (frmival.type == V4L2_FRMIVAL_TYPE_CONTINUOUS ||
            frmival.type == V4L2_FRMIVAL_TYPE_STEPWISE)
        {

            uint32_t min_num = frmival.stepwise.min.numerator;
            uint32_t min_den = frmival.stepwise.min.denominator;
            uint32_t max_num = frmival.stepwise.max.numerator;
            uint32_t max_den = frmival.stepwise.max.denominator;

            float min_fps = (float)min_den / min_num;
            float max_fps = (float)max_den / max_num;

            // Count framerates in range
            for (size_t i = 0; i < num_common_framerates; i++)
            {
                float fps = (float)common_framerates[i].denominator / common_framerates[i].numerator;
                if (fps >= min_fps && fps <= max_fps)
                {
                    rate_count++;
                }
            }
        }
    }

    if (rate_count == 0)
    {
        *framerates = NULL;
        *count = 0;
        v4l2_close(fd);
        return 0;
    }

    // Allocate framerate list
    rate_list = calloc(rate_count, sizeof(struct v4l2_framerate));
    if (!rate_list)
    {
        LOG_ERROR("Failed to allocate memory for framerate list");
        v4l2_close(fd);
        return -ENOMEM;
    }

    // Fill framerate list
    if (frmival.type == V4L2_FRMIVAL_TYPE_DISCRETE)
    {
        // Reset enumeration
        frmival.index = 0;

        int index = 0;
        while (v4l2_ioctl(fd, VIDIOC_ENUM_FRAMEINTERVALS, &frmival) == 0 && index < rate_count)
        {
            rate_list[index].numerator = frmival.discrete.numerator;
            rate_list[index].denominator = frmival.discrete.denominator;

            float fps = (float)frmival.discrete.denominator / frmival.discrete.numerator;
            // LOG_INFO("Framerate: %.2f fps (%d/%d)", fps,
            //         frmival.discrete.numerator, frmival.discrete.denominator);

            frmival.index++;
            index++;
        }
    }
    else
    {
        // For continuous/stepwise, use common framerates

        // Reset frameintervals to get min/max values
        frmival.index = 0;
        if (v4l2_ioctl(fd, VIDIOC_ENUM_FRAMEINTERVALS, &frmival) == -1)
        {
            LOG_ERROR("Failed to get frameival range");
            free(rate_list);
            v4l2_close(fd);
            return -errno;
        }

        if (frmival.type == V4L2_FRMIVAL_TYPE_CONTINUOUS ||
            frmival.type == V4L2_FRMIVAL_TYPE_STEPWISE)
        {

            uint32_t min_num = frmival.stepwise.min.numerator;
            uint32_t min_den = frmival.stepwise.min.denominator;
            uint32_t max_num = frmival.stepwise.max.numerator;
            uint32_t max_den = frmival.stepwise.max.denominator;

            float min_fps = (float)min_den / min_num;
            float max_fps = (float)max_den / max_num;

            int index = 0;
            for (size_t i = 0; i < num_common_framerates && index < rate_count; i++)
            {
                float fps = (float)common_framerates[i].denominator / common_framerates[i].numerator;
                if (fps >= min_fps && fps <= max_fps)
                {
                    rate_list[index].numerator = common_framerates[i].numerator;
                    rate_list[index].denominator = common_framerates[i].denominator;

                    // LOG_INFO("Framerate: %.2f fps (%d/%d)", fps,
                    //         common_framerates[i].numerator, common_framerates[i].denominator);

                    index++;
                }
            }
        }
    }

    v4l2_close(fd);

    *framerates = rate_list;
    *count = rate_count;

    return 0;
}
