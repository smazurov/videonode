find_package(PkgConfig REQUIRED)
find_package(Threads   REQUIRED)

# gRPC + protobuf for the daemon ↔ native control plane. pkg-config (not
# find_package CONFIG) so the heavy transitive link line resolves portably.
pkg_check_modules(GRPCPP   REQUIRED IMPORTED_TARGET grpc++)
pkg_check_modules(PROTOBUF REQUIRED IMPORTED_TARGET protobuf)

add_library(grpc_bundle INTERFACE)
target_link_libraries(grpc_bundle INTERFACE PkgConfig::GRPCPP PkgConfig::PROTOBUF)

# Abseil flag parser — explicit link needed; transitive gRPC link doesn't expose it.
find_package(absl CONFIG REQUIRED)
add_library(absl_bundle INTERFACE)
target_link_libraries(absl_bundle INTERFACE absl::flags absl::flags_parse)

find_program(GRPC_CPP_PLUGIN_PATH grpc_cpp_plugin REQUIRED)
find_program(PROTOC_PATH          protoc          REQUIRED)

message(STATUS "  grpc++:                       ${GRPCPP_VERSION}")
message(STATUS "  protobuf:                     ${PROTOBUF_VERSION}")
message(STATUS "  grpc_cpp_plugin:              ${GRPC_CPP_PLUGIN_PATH}")
message(STATUS "  protoc:                       ${PROTOC_PATH}")

pkg_check_modules(EGL    REQUIRED IMPORTED_TARGET egl)
pkg_check_modules(GLESV2 REQUIRED IMPORTED_TARGET glesv2)
pkg_check_modules(DRM    REQUIRED IMPORTED_TARGET libdrm)

# TurboJPEG — software MJPEG decode fallback when Rockchip MPP is absent (host builds).
pkg_check_modules(TURBOJPEG REQUIRED IMPORTED_TARGET libturbojpeg)

# GBM is mandatory: videonode-composer (and the GLES pipeline) always builds, so a
# missing libgbm-dev fails configure here rather than silently dropping the binary.
pkg_check_modules(GBM REQUIRED IMPORTED_TARGET gbm)
set(HAVE_GBM TRUE)

# librga — Rockchip-only CSC backend.
find_library(LIBRGA_LIB rga)
set(HAVE_RGA FALSE)
if(LIBRGA_LIB)
    set(HAVE_RGA TRUE)
    add_library(rga_iface INTERFACE)
    target_link_libraries(rga_iface INTERFACE ${LIBRGA_LIB})
endif()


# librockchip_mpp — Rockchip-only; when absent mpp_jpeg_dec is skipped.
find_library(LIBMPP_LIB rockchip_mpp)
set(HAVE_MPP FALSE)
if(LIBMPP_LIB)
    set(HAVE_MPP TRUE)
    add_library(mpp_iface INTERFACE)
    target_link_libraries(mpp_iface INTERFACE ${LIBMPP_LIB})
endif()

# libpam — only consumed by the videonode-session setuid auth helper. Optional
# so host builds without pam headers (Fedora sans pam-devel) still configure;
# the release path always has libpam0g-dev (build-deb-arm64.sh) and packaging
# fails loudly if the helper binary is missing from dist/.
find_library(LIBPAM_LIB pam)
find_path(LIBPAM_INCLUDE_DIR security/pam_appl.h)
set(HAVE_PAM FALSE)
if(LIBPAM_LIB AND LIBPAM_INCLUDE_DIR)
    set(HAVE_PAM TRUE)
    add_library(pam_iface INTERFACE)
    target_link_libraries(pam_iface INTERFACE ${LIBPAM_LIB})
    target_include_directories(pam_iface INTERFACE ${LIBPAM_INCLUDE_DIR})
endif()

# Shared EGL/GLES/GBM/DRM bundle.
add_library(gles_bundle INTERFACE)
target_link_libraries(gles_bundle INTERFACE
    PkgConfig::EGL PkgConfig::GLESV2 PkgConfig::GBM PkgConfig::DRM)

# libplacebo is mandatory: it's the composer's GPU compose + (non-RGA) CSC backend.
# Required so a missing/too-old libplacebo fails configure instead of dropping composer.
pkg_check_modules(PLACEBO REQUIRED IMPORTED_TARGET libplacebo)
set(HAVE_PLACEBO TRUE)
add_library(placebo_bundle INTERFACE)
target_link_libraries(placebo_bundle INTERFACE PkgConfig::PLACEBO)

# Vulkan loader — optional, needed by the placebo vk/host-copy probes.
pkg_check_modules(VULKAN IMPORTED_TARGET vulkan)
set(HAVE_VULKAN FALSE)
if(TARGET PkgConfig::VULKAN)
    set(HAVE_VULKAN TRUE)
endif()

message(STATUS "videonode-native deps:")
message(STATUS "  GBM (libgbm-dev):             ${GBM_VERSION}")
message(STATUS "  libturbojpeg (libjpeg-turbo): ${TURBOJPEG_VERSION}")
message(STATUS "  librga (Rockchip):            ${HAVE_RGA}")
message(STATUS "  librockchip_mpp (Rockchip):   ${HAVE_MPP}")
message(STATUS "  libpam (videonode-session):   ${HAVE_PAM}")
message(STATUS "  libplacebo:                   ${PLACEBO_VERSION}")
if(HAVE_VULKAN)
    message(STATUS "  Vulkan:                       ${VULKAN_VERSION}")
else()
    message(STATUS "  Vulkan:                       not found")
endif()

# GoogleTest — test-only, reached only when BUILD_TESTS=ON (OFF by default),
# so shipping configures never require gtest. System install, not in-tree;
# build-deb-arm64.sh adds libgtest-dev/libgmock-dev for the test modes only.
if(BUILD_TESTS)
    find_package(GTest REQUIRED)
    include(GoogleTest)
endif()
