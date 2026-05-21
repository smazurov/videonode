# src/capture/CMakeLists.txt — V4L2 capture + MJPEG decode + source-health probe.

# v4l2_capture: pure UAPI ioctl wrapper for V4L2 capture devices.
vn_add_library(v4l2_capture SOURCES v4l2_capture.cpp)

# source_probe: event-driven source-health state machine. Pure logic.
vn_add_library(source_probe
    SOURCES source_probe.cpp
    PUBLIC_DEPS v4l2_capture)

# jpeg_dec_turbo: libjpeg-turbo (TurboJPEG API) software MJPEG → NV12 decoder.
# Software fallback when MPP HW decode is unavailable (host builds).
vn_add_library(jpeg_dec_turbo
    SOURCES jpeg_dec_turbo.cpp
    PUBLIC_DEPS PkgConfig::TURBOJPEG)

# mpp_jpeg_dec: Rockchip MPP HW MJPEG → NV12 decoder. Only built when MPP
# headers + libs are present (rig path).
if(HAVE_MPP)
    vn_add_library(mpp_jpeg_dec
        SOURCES mpp_jpeg_dec.cpp
        PUBLIC_DEPS mpp_iface)
endif()
