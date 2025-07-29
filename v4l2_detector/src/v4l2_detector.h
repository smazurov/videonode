/**
 * V4L2 Camera Detector - Interface for detecting and querying V4L2 cameras
 *
 */

#pragma once

#define _GNU_SOURCE /* For strdup() */

#include <stdint.h>
#include <stddef.h>
#include <stdbool.h>
#include <time.h> /* For struct timespec */
#include <linux/videodev2.h>

#ifdef __cplusplus
extern "C"
{
#endif

    /**
     * Device information structure
     */
    struct v4l2_device_info
    {
        char *device_path; /* Device path (e.g., /dev/video0) */
        char *device_name; /* Device name */
        char *device_id;   /* Unique device identifier */
        uint32_t caps;     /* Device capabilities */
    };

    /**
     * Format information structure
     */
    struct v4l2_format_info
    {
        uint32_t pixel_format; /* V4L2 pixel format code (e.g., V4L2_PIX_FMT_YUYV) */
        char *format_name;     /* Human-readable format name */
        bool emulated;         /* Whether this format is emulated */
    };

    /**
     * Resolution structure
     */
    struct v4l2_resolution
    {
        uint32_t width;  /* Width in pixels */
        uint32_t height; /* Height in pixels */
    };

    /**
     * Framerate structure
     */
    struct v4l2_framerate
    {
        uint32_t numerator;   /* Framerate numerator */
        uint32_t denominator; /* Framerate denominator */
    };

    /**
     * Control structure
     */
    struct v4l2_control_info
    {
        uint32_t id;           /* Control ID */
        char *name;            /* Control name */
        int32_t type;          /* Control type (V4L2_CTRL_TYPE_*) */
        int32_t min;           /* Minimum value */
        int32_t max;           /* Maximum value */
        int32_t step;          /* Step size */
        int32_t default_value; /* Default value */
        uint32_t flags;        /* Control flags */
    };

    /**
     * Menu item structure
     */
    struct v4l2_menu_item
    {
        uint32_t id;    /* Item ID */
        uint32_t index; /* Item index */
        char *name;     /* Item name */
    };

    /* Core functionality */

    /**
     * Find all V4L2 camera devices on the system
     *
     * @param devices Pointer to an array of device_info structures that will be allocated
     * @param count Pointer to a variable that will receive the number of devices
     * @return 0 on success, negative error code on failure
     */
    int v4l2_find_devices(struct v4l2_device_info **devices, size_t *count);

    /**
     * Free memory allocated for device_info structures
     *
     * @param devices Array of device_info structures
     * @param count Number of devices
     */
    void v4l2_free_devices(struct v4l2_device_info *devices, size_t count);

    /* Format handling */

    /**
     * Get all supported formats for a device
     *
     * @param device_path Path to the device
     * @param formats Pointer to an array of format structures that will be allocated
     * @param count Pointer to a variable that will receive the number of formats
     * @return 0 on success, negative error code on failure
     */
    int v4l2_get_formats(const char *device_path, struct v4l2_format_info **formats, size_t *count);

    /**
     * Get all supported resolutions for a device and format
     *
     * @param device_path Path to the device
     * @param pixel_format Pixel format to get resolutions for
     * @param resolutions Pointer to an array of resolution structures that will be allocated
     * @param count Pointer to a variable that will receive the number of resolutions
     * @return 0 on success, negative error code on failure
     */
    int v4l2_get_resolutions(const char *device_path, uint32_t pixel_format,
                             struct v4l2_resolution **resolutions, size_t *count);

    /**
     * Get all supported framerates for a device, format, and resolution
     *
     * @param device_path Path to the device
     * @param pixel_format Pixel format to get framerates for
     * @param width Width of the resolution
     * @param height Height of the resolution
     * @param framerates Pointer to an array of framerate structures that will be allocated
     * @param count Pointer to a variable that will receive the number of framerates
     * @return 0 on success, negative error code on failure
     */
    int v4l2_get_framerates(const char *device_path, uint32_t pixel_format,
                            int width, int height,
                            struct v4l2_framerate **framerates, size_t *count);

    /**
     * Get all controls for a device
     *
     * @param device_path Path to the device
     * @param controls Pointer to an array of control structures that will be allocated
     * @param count Pointer to a variable that will receive the number of controls
     * @return 0 on success, negative error code on failure
     */
    int v4l2_get_controls(const char *device_path, struct v4l2_control_info **controls, size_t *count);

    /**
     * Get menu items for a menu-type control
     *
     * @param device_path Path to the device
     * @param control_id ID of the menu control
     * @param items Pointer to an array of menu item structures that will be allocated
     * @param count Pointer to a variable that will receive the number of menu items
     * @return 0 on success, negative error code on failure
     */
    int v4l2_get_menu_items(const char *device_path, uint32_t control_id, struct v4l2_menu_item **items, size_t *count);

    /* Device operations */

    /**
     * Open a V4L2 device
     *
     * @param device_path Path to the device
     * @return File descriptor on success, negative error code on failure
     */
    int v4l2_open_device(const char *device_path);

    /**
     * Close a V4L2 device
     *
     * @param device_fd File descriptor of the device
     * @return 0 on success, negative error code on failure
     */
    int v4l2_close_device(int device_fd);

    /**
     * Set a control value on a device
     *
     * @param device_path Path to the device
     * @param control_id ID of the control
     * @param value Value to set
     * @return 0 on success, negative error code on failure
     */
    int v4l2_set_ctrl_value(const char *device_path, uint32_t control_id, int32_t value);

    /**
     * Get a control value from a device
     *
     * @param device_path Path to the device
     * @param control_id ID of the control
     * @param value Pointer to a variable that will receive the value
     * @return 0 on success, negative error code on failure
     */
    int v4l2_get_ctrl_value(const char *device_path, uint32_t control_id, int32_t *value);

    /* Helper functions */

    /**
     * Convert a V4L2 pixel format code to a human-readable string
     *
     * @param format V4L2 pixel format code
     * @return Human-readable string representation of the format
     */
    const char *v4l2_format_to_string(uint32_t format);

    /**
     * Convert a framerate structure to frames per second as a float
     *
     * @param framerate Framerate structure
     * @return Frames per second as a float
     */
    float v4l2_framerate_to_fps(struct v4l2_framerate framerate);

#ifdef __cplusplus
}
#endif
