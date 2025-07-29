/**
 * V4L2 Camera Detector
 *
 * This project is inspired by and derived from the V4L2 plugin code found in OBS Studio.
 * It is licensed under All Rights Reserved.
 *
 * (Note: Replace All Rights Reserved with the actual license information.)
 */
/**
 * V4L2 Camera Detector - Main program
 *
 * This program demonstrates the use of the V4L2 detector functions
 * by listing all available V4L2 devices and their capabilities.
 */

#include "v4l2_detector.h"
#include "config.h"

#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <inttypes.h>

// Helper function to print divider
static void print_divider(char c, int width)
{
    for (int i = 0; i < width; i++)
        putchar(c);
    putchar('\n');
}

// Helper function to print section header
static void print_header(const char *title)
{
    print_divider('=', 80);
    printf("%s\n", title);
    print_divider('=', 80);
}

// Helper function to print subsection header
static void print_subheader(const char *title)
{
    print_divider('-', 60);
    printf("%s\n", title);
    print_divider('-', 60);
}

int main(int argc, char *argv[])
{
    int ret;
    struct v4l2_device_info *devices = NULL;
    size_t device_count = 0;

    printf("V4L2 Camera Detector v%s\n\n", V4L2_DETECTOR_VERSION);

    // Find all V4L2 devices
    print_header("V4L2 Camera Devices");

    ret = v4l2_find_devices(&devices, &device_count);
    if (ret < 0)
    {
        fprintf(stderr, "Failed to find V4L2 devices: %s\n", strerror(-ret));
        return 1;
    }

    if (device_count == 0)
    {
        printf("No V4L2 camera devices found.\n");
        return 0;
    }

    printf("Found %zu V4L2 camera device(s)\n\n", device_count);

    // Process each device
    for (size_t i = 0; i < device_count; i++)
    {
        printf("Device %zu:\n", i + 1);
        printf("  Path: %s\n", devices[i].device_path);
        printf("  Name: %s\n", devices[i].device_name);
        printf("  ID:   %s\n", devices[i].device_id);
        printf("  Capabilities: 0x%08x\n", devices[i].caps);

        if (devices[i].caps & V4L2_CAP_VIDEO_CAPTURE)
            printf("    - Video Capture\n");
        if (devices[i].caps & V4L2_CAP_VIDEO_OUTPUT)
            printf("    - Video Output\n");
        if (devices[i].caps & V4L2_CAP_VIDEO_OVERLAY)
            printf("    - Video Overlay\n");
        if (devices[i].caps & V4L2_CAP_STREAMING)
            printf("    - Streaming I/O\n");
        if (devices[i].caps & V4L2_CAP_READWRITE)
            printf("    - Read/Write I/O\n");

        // Get formats for this device
        struct v4l2_format_info *formats = NULL;
        size_t format_count = 0;

        print_subheader("Formats");

        ret = v4l2_get_formats(devices[i].device_path, &formats, &format_count);
        if (ret < 0)
        {
            fprintf(stderr, "Failed to get formats: %s\n", strerror(-ret));
            continue;
        }

        if (format_count == 0)
        {
            printf("No formats found.\n");
        }
        else
        {
            printf("Found %zu format(s)\n", format_count);

            for (size_t j = 0; j < format_count; j++)
            {
                char fourcc[5];
                v4l2_format_to_string(formats[j].pixel_format);
                strncpy(fourcc, v4l2_format_to_string(formats[j].pixel_format), 5);

                printf("  Format %zu: %s (%s) %s\n", j + 1,
                       formats[j].format_name,
                       fourcc,
                       formats[j].emulated ? "[Emulated]" : "");

                // Get resolutions for this format
                struct v4l2_resolution *resolutions = NULL;
                size_t resolution_count = 0;

                ret = v4l2_get_resolutions(devices[i].device_path, formats[j].pixel_format,
                                           &resolutions, &resolution_count);
                if (ret < 0)
                {
                    fprintf(stderr, "Failed to get resolutions: %s\n", strerror(-ret));
                    continue;
                }

                if (resolution_count == 0)
                {
                    printf("    No resolutions found.\n");
                }
                else
                {
                    // Find min/max resolutions
                    uint32_t min_width = resolutions[0].width;
                    uint32_t min_height = resolutions[0].height;
                    uint32_t max_width = resolutions[0].width;
                    uint32_t max_height = resolutions[0].height;

                    for (size_t k = 1; k < resolution_count; k++)
                    {
                        if (resolutions[k].width * resolutions[k].height < min_width * min_height)
                        {
                            min_width = resolutions[k].width;
                            min_height = resolutions[k].height;
                        }

                        if (resolutions[k].width * resolutions[k].height > max_width * max_height)
                        {
                            max_width = resolutions[k].width;
                            max_height = resolutions[k].height;
                        }
                    }

                    if (resolution_count == 1)
                    {
                        printf("    Resolution: %ux%u\n", min_width, min_height);
                    }
                    else
                    {
                        printf("    Resolutions: %zu options from %ux%u to %ux%u\n",
                               resolution_count, min_width, min_height, max_width, max_height);
                    }

                    // Try to find a better resolution that supports high framerates
                    struct v4l2_resolution best_res = {max_width, max_height};
                    float highest_fps = 0.0f;

                    // Sample a few common resolutions
                    const struct
                    {
                        uint32_t width;
                        uint32_t height;
                    } common_res[] = {
                        {1920, 1080}, // 1080p
                        {1280, 720},  // 720p
                        {640, 480}    // VGA
                    };

                    for (size_t r = 0; r < 3; r++)
                    {
                        // Check if this common resolution is supported
                        bool found = false;
                        for (size_t k = 0; k < resolution_count; k++)
                        {
                            if (resolutions[k].width == common_res[r].width &&
                                resolutions[k].height == common_res[r].height)
                            {
                                found = true;

                                struct v4l2_framerate *temp_rates = NULL;
                                size_t temp_count = 0;

                                ret = v4l2_get_framerates(devices[i].device_path, formats[j].pixel_format,
                                                          common_res[r].width, common_res[r].height,
                                                          &temp_rates, &temp_count);

                                if (ret >= 0 && temp_count > 0)
                                {
                                    float max_temp_fps = v4l2_framerate_to_fps(temp_rates[0]);
                                    for (size_t l = 1; l < temp_count; l++)
                                    {
                                        float fps = v4l2_framerate_to_fps(temp_rates[l]);
                                        if (fps > max_temp_fps)
                                            max_temp_fps = fps;
                                    }

                                    if (max_temp_fps > highest_fps)
                                    {
                                        highest_fps = max_temp_fps;
                                        best_res.width = common_res[r].width;
                                        best_res.height = common_res[r].height;
                                    }

                                    free(temp_rates);
                                }
                                break;
                            }
                        }
                    }

                    // If no common resolution found or no high framerate, just use max resolution
                    if (highest_fps < 5.0f)
                    {
                        best_res.width = max_width;
                        best_res.height = max_height;
                    }

                    // Get framerates for our chosen resolution
                    struct v4l2_framerate *framerates = NULL;
                    size_t framerate_count = 0;

                    ret = v4l2_get_framerates(devices[i].device_path, formats[j].pixel_format,
                                              best_res.width, best_res.height,
                                              &framerates, &framerate_count);

                    if (ret < 0)
                    {
                        fprintf(stderr, "Failed to get framerates: %s\n", strerror(-ret));
                    }
                    else if (framerate_count > 0)
                    {
                        // Find min/max framerates
                        float min_fps = v4l2_framerate_to_fps(framerates[0]);
                        float max_fps = min_fps;

                        for (size_t l = 1; l < framerate_count; l++)
                        {
                            float fps = v4l2_framerate_to_fps(framerates[l]);
                            if (fps < min_fps)
                                min_fps = fps;
                            if (fps > max_fps)
                                max_fps = fps;
                        }

                        // Only print the resolution if different from the max
                        if (best_res.width != max_width || best_res.height != max_height)
                            printf("    Optimal resolution: %ux%u\n", best_res.width, best_res.height);

                        if (framerate_count == 1)
                        {
                            printf("    Framerate: %.1f fps\n", max_fps); // Show the max fps, not min
                        }
                        else
                        {
                            printf("    Framerates: %zu options from %.1f to %.1f fps\n",
                                   framerate_count, min_fps, max_fps);
                        }

                        // Free framerates
                        free(framerates);
                    }
                }

                // Free resolutions
                free(resolutions);
            }
        }

        // Free formats
        if (formats)
        {
            for (size_t j = 0; j < format_count; j++)
            {
                free(formats[j].format_name);
            }
            free(formats);
        }

        // Get controls for this device
        struct v4l2_control_info *controls = NULL;
        size_t control_count = 0;

        print_subheader("Controls");

        ret = v4l2_get_controls(devices[i].device_path, &controls, &control_count);
        if (ret < 0)
        {
            fprintf(stderr, "Failed to get controls: %s\n", strerror(-ret));
            continue;
        }

        if (control_count == 0)
        {
            printf("No controls found.\n");
        }
        else
        {
            printf("Found %zu control(s)\n", control_count);

            for (size_t j = 0; j < control_count; j++)
            {
                const char *type_str;

                switch (controls[j].type)
                {
                case V4L2_CTRL_TYPE_INTEGER:
                    type_str = "Integer";
                    break;
                case V4L2_CTRL_TYPE_BOOLEAN:
                    type_str = "Boolean";
                    break;
                case V4L2_CTRL_TYPE_MENU:
                    type_str = "Menu";
                    break;
                case V4L2_CTRL_TYPE_INTEGER_MENU:
                    type_str = "Integer Menu";
                    break;
                case V4L2_CTRL_TYPE_BUTTON:
                    type_str = "Button";
                    break;
                default:
                    type_str = "Other";
                    break;
                }

                printf("  Control %zu: %s (ID: %u)\n", j + 1, controls[j].name, controls[j].id);
                printf("    Type: %s\n", type_str);

                if (controls[j].type == V4L2_CTRL_TYPE_INTEGER)
                {
                    printf("    Range: %d to %d (step %d)\n",
                           controls[j].min, controls[j].max, controls[j].step);
                    printf("    Default: %d\n", controls[j].default_value);
                }
                else if (controls[j].type == V4L2_CTRL_TYPE_BOOLEAN)
                {
                    printf("    Default: %s\n", controls[j].default_value ? "True" : "False");
                }
                else if (controls[j].type == V4L2_CTRL_TYPE_MENU ||
                         controls[j].type == V4L2_CTRL_TYPE_INTEGER_MENU)
                {
                    // Get menu items
                    struct v4l2_menu_item *menu_items = NULL;
                    size_t menu_item_count = 0;

                    ret = v4l2_get_menu_items(devices[i].device_path, controls[j].id,
                                              &menu_items, &menu_item_count);
                    if (ret < 0)
                    {
                        fprintf(stderr, "Failed to get menu items: %s\n", strerror(-ret));
                        continue;
                    }

                    if (menu_item_count == 0)
                    {
                        printf("    No menu items found.\n");
                    }
                    else
                    {
                        printf("    Default: %d\n", controls[j].default_value);
                        printf("    Menu items:\n");

                        for (size_t k = 0; k < menu_item_count; k++)
                        {
                            printf("      %u: %s%s\n", menu_items[k].index, menu_items[k].name,
                                   menu_items[k].index == (uint32_t)controls[j].default_value ? " (Default)" : "");
                        }

                        // Free menu items
                        if (menu_items)
                        {
                            for (size_t k = 0; k < menu_item_count; k++)
                            {
                                free(menu_items[k].name);
                            }
                            free(menu_items);
                        }
                    }
                }
            }
        }

        // Free controls
        if (controls)
        {
            for (size_t j = 0; j < control_count; j++)
            {
                free(controls[j].name);
            }
            free(controls);
        }

        printf("\n");
    }

    // Free devices
    v4l2_free_devices(devices, device_count);

    return 0;
}
