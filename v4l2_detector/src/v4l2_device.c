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

    // Single-pass enumeration with dynamic allocation
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

        uint32_t caps = (video_cap.capabilities & V4L2_CAP_DEVICE_CAPS) ? video_cap.device_caps : video_cap.capabilities;

        if (!(caps & V4L2_CAP_VIDEO_CAPTURE))
        {
            LOG_INFO("%s does not support video capture", device_path);
            v4l2_close(fd);
            continue;
        }

        // Allocate or grow device list
        struct v4l2_device_info *new_list = realloc(dev_list, (dev_count + 1) * sizeof(struct v4l2_device_info));
        if (!new_list)
        {
            LOG_ERROR("Failed to allocate memory for device list");
            free(dev_list);
            closedir(dirp);
            v4l2_close(fd);
            return -ENOMEM;
        }
        dev_list = new_list;

        // Fill device info
        dev_list[dev_count].device_path = strdup(device_path);
        dev_list[dev_count].device_name = strdup((char *)video_cap.card);

        // Find the stable by-id symlink for this device
        char index_path[512];
        snprintf(index_path, sizeof(index_path), "/sys/class/video4linux/%s/index", dp->d_name);
        FILE *index_file = fopen(index_path, "r");
        int index_value = 0;
        if (index_file) {
            fscanf(index_file, "%d", &index_value);
            fclose(index_file);
        }

        // Look for the corresponding symlink in /dev/v4l/by-id/
        char *stable_id = NULL;
        DIR *by_id_dir = opendir("/dev/v4l/by-id");
        if (by_id_dir) {
            struct dirent *link_entry;
            while ((link_entry = readdir(by_id_dir)) != NULL) {
                if (link_entry->d_type != DT_LNK)
                    continue;
                
                // Check if this symlink points to our device
                char link_path[512];
                char target[256];
                snprintf(link_path, sizeof(link_path), "/dev/v4l/by-id/%s", link_entry->d_name);
                
                ssize_t len = readlink(link_path, target, sizeof(target) - 1);
                if (len > 0) {
                    target[len] = '\0';
                    // Extract just the video device name from the target
                    char *video_name = strrchr(target, '/');
                    if (video_name) {
                        video_name++; // Skip the '/'
                    } else {
                        video_name = target;
                    }
                    
                    // Check if this symlink points to our current device
                    if (strcmp(video_name, dp->d_name) == 0) {
                        // Check if it has the right index suffix
                        char expected_suffix[32];
                        snprintf(expected_suffix, sizeof(expected_suffix), "-video-index%d", index_value);
                        if (strstr(link_entry->d_name, expected_suffix)) {
                            stable_id = strdup(link_entry->d_name);
                            break;
                        }
                    }
                }
            }
            closedir(by_id_dir);
        }

        // If we found a stable by-id symlink, use it; otherwise fall back to bus_info with index
        if (stable_id) {
            dev_list[dev_count].device_id = stable_id;
            LOG_INFO("Found device '%s' at %s with stable ID: %s", video_cap.card, device_path, stable_id);
        } else {
            // Fallback: create a synthetic ID based on bus_info and index
            char fallback_id[512];
            // Add platform- prefix for non-USB devices to match by-path symlinks
            if (strncmp(video_cap.bus_info, "usb-", 4) == 0) {
                snprintf(fallback_id, sizeof(fallback_id), "%s-video-index%d", video_cap.bus_info, index_value);
            } else {
                snprintf(fallback_id, sizeof(fallback_id), "platform-%s-video-index%d", video_cap.bus_info, index_value);
            }
            dev_list[dev_count].device_id = strdup(fallback_id);
            LOG_INFO("Found device '%s' at %s with fallback ID: %s", video_cap.card, device_path, fallback_id);
        }

        dev_list[dev_count].caps = caps;

        dev_count++;
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
