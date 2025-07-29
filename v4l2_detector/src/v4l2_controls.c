/**
 * V4L2 Camera Detector - Control Functions
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

// Implementations of control functions

int v4l2_get_controls(const char *device_path, struct v4l2_control_info **controls, size_t *count)
{
    int fd;
    int ctrl_count = 0;
    struct v4l2_control_info *ctrl_list = NULL;

    fd = v4l2_open(device_path, O_RDWR | O_NONBLOCK);
    if (fd == -1)
    {
        LOG_ERROR("Unable to open %s", device_path);
        return -errno;
    }

    // First pass: count controls
    struct v4l2_queryctrl qctrl;
    memset(&qctrl, 0, sizeof(qctrl));
    qctrl.id = V4L2_CTRL_FLAG_NEXT_CTRL;

    while (v4l2_ioctl(fd, VIDIOC_QUERYCTRL, &qctrl) == 0)
    {
        // Skip disabled controls
        if (qctrl.flags & V4L2_CTRL_FLAG_DISABLED)
        {
            qctrl.id |= V4L2_CTRL_FLAG_NEXT_CTRL;
            continue;
        }

        ctrl_count++;
        qctrl.id |= V4L2_CTRL_FLAG_NEXT_CTRL;
    }

    // Reset and try the older enumeration method if no controls were found
    if (ctrl_count == 0)
    {
        for (unsigned int id = V4L2_CID_BASE; id < V4L2_CID_LASTP1; id++)
        {
            qctrl.id = id;

            if (v4l2_ioctl(fd, VIDIOC_QUERYCTRL, &qctrl) == 0)
            {
                // Skip disabled controls
                if (qctrl.flags & V4L2_CTRL_FLAG_DISABLED)
                    continue;

                ctrl_count++;
            }
        }

        // Try private controls
        for (unsigned int id = V4L2_CID_PRIVATE_BASE;; id++)
        {
            qctrl.id = id;

            if (v4l2_ioctl(fd, VIDIOC_QUERYCTRL, &qctrl) == 0)
            {
                // Skip disabled controls
                if (qctrl.flags & V4L2_CTRL_FLAG_DISABLED)
                    continue;

                ctrl_count++;
            }
            else
            {
                break;
            }
        }
    }

    if (ctrl_count == 0)
    {
        *controls = NULL;
        *count = 0;
        v4l2_close(fd);
        return 0;
    }

    // Allocate control list
    ctrl_list = calloc(ctrl_count, sizeof(struct v4l2_control_info));
    if (!ctrl_list)
    {
        LOG_ERROR("Failed to allocate memory for control list");
        v4l2_close(fd);
        return -ENOMEM;
    }

    // Second pass: fill control list
    memset(&qctrl, 0, sizeof(qctrl));
    qctrl.id = V4L2_CTRL_FLAG_NEXT_CTRL;

    int index = 0;
    bool using_next_ctrl = true;

    while (index < ctrl_count)
    {
        if (using_next_ctrl)
        {
            // Try the extended control enumeration
            if (v4l2_ioctl(fd, VIDIOC_QUERYCTRL, &qctrl) != 0)
            {
                using_next_ctrl = false;
                qctrl.id = V4L2_CID_BASE;
                continue;
            }

            // Skip disabled controls
            if (qctrl.flags & V4L2_CTRL_FLAG_DISABLED)
            {
                qctrl.id |= V4L2_CTRL_FLAG_NEXT_CTRL;
                continue;
            }

            // Store control info
            ctrl_list[index].id = qctrl.id;
            ctrl_list[index].name = strdup((char *)qctrl.name);
            ctrl_list[index].type = qctrl.type;
            ctrl_list[index].min = qctrl.minimum;
            ctrl_list[index].max = qctrl.maximum;
            ctrl_list[index].step = qctrl.step;
            ctrl_list[index].default_value = qctrl.default_value;
            ctrl_list[index].flags = qctrl.flags;

            LOG_INFO("Control: %s (ID: %d, Type: %d, Range: %d-%d, Step: %d, Default: %d)",
                     qctrl.name, qctrl.id, qctrl.type, qctrl.minimum, qctrl.maximum,
                     qctrl.step, qctrl.default_value);

            qctrl.id |= V4L2_CTRL_FLAG_NEXT_CTRL;
            index++;
        }
        else
        {
            // Use the legacy enumeration for older drivers
            bool found = false;

            // Try standard controls
            if (qctrl.id >= V4L2_CID_BASE && qctrl.id < V4L2_CID_LASTP1)
            {
                if (v4l2_ioctl(fd, VIDIOC_QUERYCTRL, &qctrl) == 0 &&
                    !(qctrl.flags & V4L2_CTRL_FLAG_DISABLED))
                {
                    found = true;
                }
                qctrl.id++;
            }
            // Try private controls if we've exhausted standard ones
            else if (qctrl.id == V4L2_CID_LASTP1)
            {
                qctrl.id = V4L2_CID_PRIVATE_BASE;
            }
            // Query private controls
            else if (qctrl.id >= V4L2_CID_PRIVATE_BASE)
            {
                if (v4l2_ioctl(fd, VIDIOC_QUERYCTRL, &qctrl) == 0 &&
                    !(qctrl.flags & V4L2_CTRL_FLAG_DISABLED))
                {
                    found = true;
                }
                else
                {
                    // No more private controls
                    break;
                }
                qctrl.id++;
            }

            if (found)
            {
                // Store control info
                ctrl_list[index].id = qctrl.id - 1; // Adjust for the increment
                ctrl_list[index].name = strdup((char *)qctrl.name);
                ctrl_list[index].type = qctrl.type;
                ctrl_list[index].min = qctrl.minimum;
                ctrl_list[index].max = qctrl.maximum;
                ctrl_list[index].step = qctrl.step;
                ctrl_list[index].default_value = qctrl.default_value;
                ctrl_list[index].flags = qctrl.flags;

                LOG_INFO("Control: %s (ID: %d, Type: %d, Range: %d-%d, Step: %d, Default: %d)",
                         qctrl.name, qctrl.id - 1, qctrl.type, qctrl.minimum, qctrl.maximum,
                         qctrl.step, qctrl.default_value);

                index++;
            }
        }
    }

    v4l2_close(fd);

    *controls = ctrl_list;
    *count = ctrl_count;

    return 0;
}

int v4l2_get_menu_items(const char *device_path, uint32_t control_id, struct v4l2_menu_item **items, size_t *count)
{
    int fd;
    int item_count = 0;
    struct v4l2_menu_item *item_list = NULL;

    fd = v4l2_open(device_path, O_RDWR | O_NONBLOCK);
    if (fd == -1)
    {
        LOG_ERROR("Unable to open %s", device_path);
        return -errno;
    }

    // First, get control info to determine range
    struct v4l2_queryctrl qctrl;
    memset(&qctrl, 0, sizeof(qctrl));
    qctrl.id = control_id;

    if (v4l2_ioctl(fd, VIDIOC_QUERYCTRL, &qctrl) != 0)
    {
        LOG_ERROR("Failed to query control %d", control_id);
        v4l2_close(fd);
        return -errno;
    }

    if (qctrl.type != V4L2_CTRL_TYPE_MENU && qctrl.type != V4L2_CTRL_TYPE_INTEGER_MENU)
    {
        LOG_ERROR("Control %d is not a menu type", control_id);
        v4l2_close(fd);
        return -EINVAL;
    }

    // First pass: count menu items
    struct v4l2_querymenu qmenu;
    memset(&qmenu, 0, sizeof(qmenu));
    qmenu.id = control_id;

    for (qmenu.index = qctrl.minimum; qmenu.index <= (uint32_t)qctrl.maximum; qmenu.index++)
    {
        if (v4l2_ioctl(fd, VIDIOC_QUERYMENU, &qmenu) == 0)
        {
            item_count++;
        }
    }

    if (item_count == 0)
    {
        *items = NULL;
        *count = 0;
        v4l2_close(fd);
        return 0;
    }

    // Allocate menu item list
    item_list = calloc(item_count, sizeof(struct v4l2_menu_item));
    if (!item_list)
    {
        LOG_ERROR("Failed to allocate memory for menu item list");
        v4l2_close(fd);
        return -ENOMEM;
    }

    // Second pass: fill menu item list
    memset(&qmenu, 0, sizeof(qmenu));
    qmenu.id = control_id;

    int index = 0;
    for (qmenu.index = qctrl.minimum; qmenu.index <= (uint32_t)qctrl.maximum && index < item_count; qmenu.index++)
    {
        if (v4l2_ioctl(fd, VIDIOC_QUERYMENU, &qmenu) == 0)
        {
            item_list[index].id = qmenu.id;
            item_list[index].index = qmenu.index;

            if (qctrl.type == V4L2_CTRL_TYPE_MENU)
            {
                item_list[index].name = strdup((char *)qmenu.name);
                LOG_INFO("Menu item: %s (ID: %d, Index: %d)", qmenu.name, qmenu.id, qmenu.index);
            }
            else
            {
                // For integer menu, convert value to string
                char value_str[32];
                snprintf(value_str, sizeof(value_str), "%d", qmenu.value);
                item_list[index].name = strdup(value_str);
                LOG_INFO("Menu item: %d (ID: %d, Index: %d)", qmenu.value, qmenu.id, qmenu.index);
            }

            index++;
        }
    }

    v4l2_close(fd);

    *items = item_list;
    *count = item_count;

    return 0;
}

int v4l2_set_ctrl_value(const char *device_path, uint32_t control_id, int32_t value)
{
    int fd = v4l2_open(device_path, O_RDWR | O_NONBLOCK);
    if (fd == -1)
    {
        LOG_ERROR("Unable to open %s: %s", device_path, strerror(errno));
        return -errno;
    }

    struct v4l2_control control;
    control.id = control_id;
    control.value = value;

    int ret = v4l2_ioctl(fd, VIDIOC_S_CTRL, &control);
    if (ret != 0)
    {
        LOG_ERROR("Failed to set control %d to value %d: %s",
                  control_id, value, strerror(errno));
        v4l2_close(fd);
        return -errno;
    }

    v4l2_close(fd);
    return 0;
}

int v4l2_get_ctrl_value(const char *device_path, uint32_t control_id, int32_t *value)
{
    int fd = v4l2_open(device_path, O_RDWR | O_NONBLOCK);
    if (fd == -1)
    {
        LOG_ERROR("Unable to open %s: %s", device_path, strerror(errno));
        return -errno;
    }

    struct v4l2_control control;
    control.id = control_id;

    int ret = v4l2_ioctl(fd, VIDIOC_G_CTRL, &control);
    if (ret != 0)
    {
        LOG_ERROR("Failed to get control %d: %s", control_id, strerror(errno));
        v4l2_close(fd);
        return -errno;
    }

    *value = control.value;

    v4l2_close(fd);
    return 0;
}
