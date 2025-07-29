/**
 * V4L2 Camera Detector - Utility Functions
 *
 * This project is inspired by and derived from the V4L2 plugin code found in OBS Studio.
 * It is licensed under All Rights Reserved.
 */

#include "v4l2_utils.h"

// Implementations of utility functions

// Helper function to convert a v4l2 fourcc code to a string
void v4l2_fourcc_to_str(uint32_t fourcc, char *str)
{
    str[0] = fourcc & 0xFF;
    str[1] = (fourcc >> 8) & 0xFF;
    str[2] = (fourcc >> 16) & 0xFF;
    str[3] = (fourcc >> 24) & 0xFF;
    str[4] = '\0';
}

int v4l2_open_device(const char *device_path)
{
    int fd = v4l2_open(device_path, O_RDWR | O_NONBLOCK);
    if (fd == -1)
    {
        LOG_ERROR("Unable to open %s: %s", device_path, strerror(errno));
        return -errno;
    }

    return fd;
}

int v4l2_close_device(int device_fd)
{
    if (device_fd < 0)
        return 0;

    int ret = v4l2_close(device_fd);
    if (ret != 0)
    {
        LOG_ERROR("Failed to close device: %s", strerror(errno));
        return -errno;
    }

    return 0;
}

const char *v4l2_format_to_string(uint32_t format)
{
    static char str[8];

    v4l2_fourcc_to_str(format, str);

    return str;
}

float v4l2_framerate_to_fps(struct v4l2_framerate framerate)
{
    if (framerate.numerator == 0)
        return 0.0f;

    return (float)framerate.denominator / framerate.numerator;
}

int64_t v4l2_pack_tuple(int32_t a, int32_t b)
{
    return ((uint64_t)a << 32) | ((uint64_t)b & 0xffffffff);
}

void v4l2_unpack_tuple(int32_t *a, int32_t *b, int64_t packed)
{
    // Since we changed from 32 to 64 bits, handle old values too.
    if ((packed & 0xffffffff00000000) == 0)
    {
        *a = (int32_t)(packed >> 16);
        *b = (int32_t)(packed & 0xffff);
    }
    else
    {
        *a = (int32_t)(packed >> 32);
        *b = (int32_t)(packed & 0xffffffff);
    }
}
