cmake_minimum_required(VERSION 3.15)
project(v4l2_detector VERSION 0.1.0 LANGUAGES C)

# C standard
set(CMAKE_C_STANDARD 11)
set(CMAKE_C_STANDARD_REQUIRED ON)
set(CMAKE_C_EXTENSIONS OFF)

# Build type default to Release if not specified
if(NOT CMAKE_BUILD_TYPE)
  set(CMAKE_BUILD_TYPE Release)
endif()

# Options
option(ENABLE_UDEV "Build with UDEV support for device hot-plugging" ON)

# Find required libraries
find_package(PkgConfig REQUIRED)
pkg_check_modules(LIBV4L2 REQUIRED libv4l2)
pkg_check_modules(FFMPEG REQUIRED libavcodec libavutil libavformat)




# Find libudev if enabled
if(ENABLE_UDEV)
  pkg_check_modules(LIBUDEV REQUIRED libudev)
endif()

# Configure header
configure_file(
  ${CMAKE_CURRENT_SOURCE_DIR}/src/config.h.in
  ${CMAKE_CURRENT_BINARY_DIR}/include/config.h
)


# Source files for the library
set(SOURCES_LIB
    src/v4l2_device.c
    src/v4l2_formats.c
    src/v4l2_controls.c
    src/v4l2_utils.c
    src/v4l2_signal.c
)

# Add the library target
add_library(v4l2_detector_lib STATIC ${SOURCES_LIB})

# Source files for the executable
set(SOURCES_EXEC
    src/main.c
)

# Add the executable target
add_executable(v4l2_detector ${SOURCES_EXEC})

# Compile definitions and include directories for both targets
target_compile_definitions(v4l2_detector PRIVATE V4L2_DETECTOR_SUPPRESS_INFO)
target_compile_definitions(v4l2_detector_lib PRIVATE V4L2_DETECTOR_SUPPRESS_INFO) # Apply to library too

target_compile_definitions(v4l2_detector PRIVATE HAVE_UDEV)
target_compile_definitions(v4l2_detector_lib PRIVATE HAVE_UDEV) # Apply to library too


target_include_directories(v4l2_detector PRIVATE
    ${CMAKE_CURRENT_SOURCE_DIR}/src
    ${CMAKE_CURRENT_BINARY_DIR}/include # For config.h
    ${LIBV4L2_INCLUDE_DIRS}
    ${FFMPEG_INCLUDE_DIRS}
)

target_include_directories(v4l2_detector_lib PRIVATE # Apply to library
    ${CMAKE_CURRENT_SOURCE_DIR}/src
    ${CMAKE_CURRENT_BINARY_DIR}/include # For config.h
    ${LIBV4L2_INCLUDE_DIRS}
    ${FFMPEG_INCLUDE_DIRS}
)


if(ENABLE_UDEV)
    target_include_directories(v4l2_detector PRIVATE ${LIBUDEV_INCLUDE_DIRS})
    target_include_directories(v4l2_detector_lib PRIVATE ${LIBUDEV_INCLUDE_DIRS}) # Apply to library
endif()

# Link the executable against the library and other dependencies
target_link_libraries(v4l2_detector
    v4l2_detector_lib # Link against the local library
    ${LIBV4L2_LIBRARIES}
    ${FFMPEG_LIBRARIES}
    ${LIBUDEV_LIBRARIES}
)

# The library target does not need to link against the external dependencies
# as they are handled by the executable linking or are header-only.
# If the library itself needed to call functions from these libs,
# target_link_libraries(v4l2_detector_lib ...) would be needed.
# Based on the C code structure, it seems the main executable uses these libs directly.


# Installation - Remove or comment out as we are not installing the executable
# install(TARGETS v4l2_detector DESTINATION bin)
