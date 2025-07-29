/**
 * V4L2 Camera Detector - Device Functions
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
#include <dirent.h>
#include <unistd.h>
#include <errno.h>
#include <sys/types.h>
#include <sys/ioctl.h>
#include <sys/time.h>
#include <time.h>
#include <linux/videodev2.h>
#include <libv4l2.h>

#ifdef HAVE_UDEV
#include <libudev.h>
#endif

// Implementations of device functions

int v4l2_find_devices(struct v4l2_device_info **devices, size_t *count)
{
    DIR *dirp;
    struct dirent *dp;
    int dev_count = 0;
    struct v4l2_device_info *dev_list = NULL;

#ifdef __FreeBSD__
    dirp = opendir("/dev");
#else
    dirp = opendir("/sys/class/video4linux");
#endif

    if (!dirp)
    {
        LOG_ERROR("Failed to open video device directory");
        return -ENOENT;
    }

    // First pass: count devices
    while ((dp = readdir(dirp)) != NULL)
    {
        int fd;
        char device_path[256];

#ifdef __FreeBSD__
        if (strstr(dp->d_name, "video") == NULL)
            continue;
#endif

        if (dp->d_type == DT_DIR)
            continue;

        snprintf(device_path, sizeof(device_path), "/dev/%s", dp->d_name);

        fd = v4l2_open(device_path, O_RDWR | O_NONBLOCK);
        if (fd == -1)
        {
            LOG_INFO("Unable to open %s", device_path);
            continue;
        }

        struct v4l2_capability video_cap;
        if (v4l2_ioctl(fd, VIDIOC_QUERYCAP, &video_cap) == -1)
        {
            LOG_INFO("Failed to query capabilities for %s", device_path);
            v4l2_close(fd);
            continue;
        }

#ifndef V4L2_CAP_DEVICE_CAPS
        uint32_t caps = video_cap.capabilities;
#else
        /* ... since Linux 3.3 */
        uint32_t caps = (video_cap.capabilities & V4L2_CAP_DEVICE_CAPS) ? video_cap.device_caps : video_cap.capabilities;
#endif

        if (!(caps & V4L2_CAP_VIDEO_CAPTURE))
        {
            LOG_INFO("%s does not support video capture", device_path);
            v4l2_close(fd);
            continue;
        }

        dev_count++;
        v4l2_close(fd);
    }

    // Allocate device list
    if (dev_count > 0)
    {
        dev_list = calloc(dev_count, sizeof(struct v4l2_device_info));
        if (!dev_list)
        {
            LOG_ERROR("Failed to allocate memory for device list");
            closedir(dirp);
            return -ENOMEM;
        }
    }
    else
    {
        *devices = NULL;
        *count = 0;
        closedir(dirp);
        return 0;
    }

    // Reset directory position
    rewinddir(dirp);

    // Second pass: fill device list
    int index = 0;
    while ((dp = readdir(dirp)) != NULL && index < dev_count)
    {
        int fd;
        char device_path[256];

#ifdef __FreeBSD__
        if (strstr(dp->d_name, "video") == NULL)
            continue;
#endif

        if (dp->d_type == DT_DIR)
            continue;

        snprintf(device_path, sizeof(device_path), "/dev/%s", dp->d_name);

        fd = v4l2_open(device_path, O_RDWR | O_NONBLOCK);
        if (fd == -1)
        {
            continue;
        }

        struct v4l2_capability video_cap;
        if (v4l2_ioctl(fd, VIDIOC_QUERYCAP, &video_cap) == -1)
        {
            v4l2_close(fd);
            continue;
        }

#ifndef V4L2_CAP_DEVICE_CAPS
        uint32_t caps = video_cap.capabilities;
#else
        uint32_t caps = (video_cap.capabilities & V4L2_CAP_DEVICE_CAPS) ? video_cap.device_caps : video_cap.capabilities;
#endif

        if (!(caps & V4L2_CAP_VIDEO_CAPTURE))
        {
            v4l2_close(fd);
            continue;
        }

        // Fill device info
        dev_list[index].device_path = strdup(device_path);
        dev_list[index].device_name = strdup((char *)video_cap.card);

        // Use just the bus info as stable device ID
        dev_list[index].device_id = strdup((char *)video_cap.bus_info);

        dev_list[index].caps = caps;

        LOG_INFO("Found device '%s' at %s", video_cap.card, device_path);

        index++;
        v4l2_close(fd);
    }

    closedir(dirp);

    *devices = dev_list;
    *count = dev_count;

    return 0;
}

void v4l2_free_devices(struct v4l2_device_info *devices, size_t count)
{
    if (!devices)
        return;

    for (size_t i = 0; i < count; i++)
    {
        free(devices[i].device_path);
        free(devices[i].device_name);
        free(devices[i].device_id);
    }

    free(devices);
}
