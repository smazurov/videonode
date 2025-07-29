#pragma once

#include <stdio.h>
#include <stdint.h>
#include <string.h>
#include <fcntl.h>
#include <unistd.h>
#include <errno.h>
#include <libv4l2.h>
#include "v4l2_detector.h" // For struct v4l2_framerate
#include <dirent.h>        // For DT_DIR defines

// Logging macros
#define LOG_ERROR(msg, ...) fprintf(stderr, "[ERROR] " msg "\n", ##__VA_ARGS__)
#ifdef V4L2_DETECTOR_SUPPRESS_INFO
#define LOG_INFO(msg, ...) (void)0
#else
#define LOG_INFO(msg, ...) fprintf(stdout, "[INFO] " msg "\n", ##__VA_ARGS__)
#endif
#define LOG_DEBUG(msg, ...) fprintf(stdout, "[DEBUG] " msg "\n", ##__VA_ARGS__)

// Dirent definitions
#ifndef DT_DIR
#define DT_UNKNOWN 0
#define DT_FIFO 1
#define DT_CHR 2
#define DT_DIR 4
#define DT_BLK 6
#define DT_REG 8
#define DT_LNK 10
#define DT_SOCK 12
#endif

// Function declarations
void v4l2_fourcc_to_str(uint32_t fourcc, char *str);
int64_t v4l2_pack_tuple(int32_t a, int32_t b);
void v4l2_unpack_tuple(int32_t *a, int32_t *b, int64_t packed);
int v4l2_open_device(const char *device_path);
int v4l2_close_device(int device_fd);
const char *v4l2_format_to_string(uint32_t format);
float v4l2_framerate_to_fps(struct v4l2_framerate framerate);
