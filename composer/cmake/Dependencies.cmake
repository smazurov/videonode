# Dependencies.cmake — resolve all third-party libs and set HAVE_* flags.
# When a Rockchip lib is missing, fall back to vendored stub headers under
# third_party/rockchip-stubs/ so the tree still compiles on a generic Linux
# dev machine. Stub calls log + return failure at runtime.

find_package(PkgConfig REQUIRED)
find_package(Threads   REQUIRED)

# Required on every Linux box with Mesa userspace + libdrm; transitively
# pulled in by tests / host-side libs.
pkg_check_modules(EGL    REQUIRED IMPORTED_TARGET egl)
pkg_check_modules(GLESV2 REQUIRED IMPORTED_TARGET glesv2)
pkg_check_modules(DRM    REQUIRED IMPORTED_TARGET libdrm)

# libjpeg-turbo's TurboJPEG API. The videonode-source MJPEG path uses it
# as the software fallback when Rockchip MPP isn't available (i.e. host
# builds). Required — no silent stub fallback. Fedora: turbojpeg-devel,
# Debian/Ubuntu: libturbojpeg0-dev.
pkg_check_modules(TURBOJPEG REQUIRED IMPORTED_TARGET libturbojpeg)

# GBM ships separately on most distros. Without it the EGL platform GBM
# path is unavailable, so the videonode-composer binary + GLES probes are
# skipped. Tests + UAPI libs still build.
pkg_check_modules(GBM IMPORTED_TARGET gbm)
set(HAVE_GBM FALSE)
if(TARGET PkgConfig::GBM)
    set(HAVE_GBM TRUE)
endif()

# librga + librockchip_mpp are Rockchip-only. Resolve by library name.
find_library(LIBRGA_LIB rga)
find_library(LIBMPP_LIB rockchip_mpp)
set(HAVE_RGA FALSE)
set(HAVE_MPP FALSE)
set(USING_ROCKCHIP_STUBS FALSE)

if(LIBRGA_LIB)
    set(HAVE_RGA TRUE)
    add_library(rga_iface INTERFACE)
    target_link_libraries(rga_iface INTERFACE ${LIBRGA_LIB})
endif()
if(LIBMPP_LIB)
    set(HAVE_MPP TRUE)
    add_library(mpp_iface INTERFACE)
    target_link_libraries(mpp_iface INTERFACE ${LIBMPP_LIB})
endif()

if(NOT HAVE_RGA OR NOT HAVE_MPP)
    set(USING_ROCKCHIP_STUBS TRUE)
    # One stub object library covers both rga and mpp. Built only when at
    # least one of the real libs is missing.
    add_library(rockchip_stubs OBJECT
        ${CMAKE_SOURCE_DIR}/third_party/rockchip-stubs/stubs.c)
    target_include_directories(rockchip_stubs PUBLIC
        ${CMAKE_SOURCE_DIR}/third_party/rockchip-stubs)

    if(NOT HAVE_RGA)
        set(HAVE_RGA TRUE)        # compile path enabled via stubs
        add_library(rga_iface INTERFACE)
        target_link_libraries(rga_iface INTERFACE rockchip_stubs)
        target_include_directories(rga_iface INTERFACE
            ${CMAKE_SOURCE_DIR}/third_party/rockchip-stubs)
    endif()
    if(NOT HAVE_MPP)
        set(HAVE_MPP TRUE)
        add_library(mpp_iface INTERFACE)
        target_link_libraries(mpp_iface INTERFACE rockchip_stubs)
        target_include_directories(mpp_iface INTERFACE
            ${CMAKE_SOURCE_DIR}/third_party/rockchip-stubs)
    endif()
endif()

# Shared interface bundle for any target that wants EGL/GLES/GBM/DRM in one
# go. Optional — only created when HAVE_GBM.
if(HAVE_GBM)
    add_library(gles_bundle INTERFACE)
    target_link_libraries(gles_bundle INTERFACE
        PkgConfig::EGL PkgConfig::GLESV2 PkgConfig::GBM PkgConfig::DRM)
endif()

message(STATUS "videonode-native deps:")
message(STATUS "  GBM (libgbm-dev):           ${HAVE_GBM}")
message(STATUS "  libturbojpeg (libjpeg-turbo): ${TURBOJPEG_VERSION}")
if(USING_ROCKCHIP_STUBS)
    message(STATUS "  Rockchip libs (rga + mpp):  STUBBED (host build)")
    message(STATUS "    -> code compiles; runtime calls log + return failure")
else()
    message(STATUS "  librga (Rockchip):          ${HAVE_RGA}")
    message(STATUS "  librockchip_mpp (Rockchip): ${HAVE_MPP}")
endif()
if(NOT HAVE_GBM)
    message(STATUS "  -> GLES probes + videonode-composer binary will be SKIPPED")
endif()
