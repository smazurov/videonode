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

# Without GBM the videonode-composer binary + GLES probes are skipped.
pkg_check_modules(GBM IMPORTED_TARGET gbm)
set(HAVE_GBM FALSE)
if(TARGET PkgConfig::GBM)
    set(HAVE_GBM TRUE)
endif()

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

# Shared EGL/GLES/GBM/DRM bundle; only created when HAVE_GBM.
if(HAVE_GBM)
    add_library(gles_bundle INTERFACE)
    target_link_libraries(gles_bundle INTERFACE
        PkgConfig::EGL PkgConfig::GLESV2 PkgConfig::GBM PkgConfig::DRM)
endif()

# libplacebo — optional; enables the placebo CSC backend + evaluation probes (#9).
pkg_check_modules(PLACEBO IMPORTED_TARGET libplacebo)
set(HAVE_PLACEBO FALSE)
if(TARGET PkgConfig::PLACEBO)
    set(HAVE_PLACEBO TRUE)
    add_library(placebo_bundle INTERFACE)
    target_link_libraries(placebo_bundle INTERFACE PkgConfig::PLACEBO)
endif()

# Vulkan loader — optional, needed by the placebo vk/host-copy probes.
pkg_check_modules(VULKAN IMPORTED_TARGET vulkan)
set(HAVE_VULKAN FALSE)
if(TARGET PkgConfig::VULKAN)
    set(HAVE_VULKAN TRUE)
endif()

message(STATUS "videonode-native deps:")
message(STATUS "  GBM (libgbm-dev):             ${HAVE_GBM}")
message(STATUS "  libturbojpeg (libjpeg-turbo): ${TURBOJPEG_VERSION}")
message(STATUS "  librga (Rockchip):            ${HAVE_RGA}")
message(STATUS "  librockchip_mpp (Rockchip):   ${HAVE_MPP}")
if(HAVE_PLACEBO)
    message(STATUS "  libplacebo:                   ${PLACEBO_VERSION}")
else()
    message(STATUS "  libplacebo:                   not found")
endif()
if(HAVE_VULKAN)
    message(STATUS "  Vulkan:                       ${VULKAN_VERSION}")
else()
    message(STATUS "  Vulkan:                       not found")
endif()
if(NOT HAVE_GBM)
    message(STATUS "  -> GLES probes + videonode-composer binary will be SKIPPED")
endif()
if(NOT HAVE_RGA AND NOT HAVE_PLACEBO)
    message(WARNING
        "  No CSC backend available (HAVE_RGA=off, HAVE_PLACEBO=off). "
        "videonode-source will still build, but non-NV12 V4L2 sources will "
        "be dropped at runtime. Install librga (RK3588) or libplacebo-devel "
        "(generic Linux) to enable a real backend.")
endif()

# GoogleTest — test-only, reached only when BUILD_TESTS=ON (OFF by default),
# so shipping configures never require gtest. System install, not in-tree;
# build-deb-arm64.sh adds libgtest-dev/libgmock-dev for the test modes only.
if(BUILD_TESTS)
    find_package(GTest REQUIRED)
    include(GoogleTest)
endif()
