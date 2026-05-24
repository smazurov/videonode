find_package(PkgConfig REQUIRED)
find_package(Threads   REQUIRED)

# gRPC + protobuf for the daemon ↔ native control plane. Matches the
# project's pkg-config convention used for EGL / GLES / DRM above.
# Debian trixie ships gRPC 1.51.1 (libgrpc++-dev) and protobuf 3.21.12
# (libprotobuf-dev); Fedora 43+ has grpc-devel / grpc-plugins /
# protobuf-devel. The Apache-libgrpc++ pkg-config link line is heavy
# (transitive abseil/cares/re2/ssl/etc.) — pkg_check_modules captures
# that automatically; find_package(gRPC CONFIG) does the same but
# requires the CMake config files to be in the right place, which varies.
pkg_check_modules(GRPCPP   REQUIRED IMPORTED_TARGET grpc++)
pkg_check_modules(PROTOBUF REQUIRED IMPORTED_TARGET protobuf)

add_library(grpc_bundle INTERFACE)
target_link_libraries(grpc_bundle INTERFACE PkgConfig::GRPCPP PkgConfig::PROTOBUF)

# Abseil's flag parser. Abseil is already pulled in transitively by gRPC's
# pkg-config link line, but the symbols we use (ABSL_FLAG, absl::ParseCommandLine,
# absl::SetProgramUsageMessage) require an explicit link against absl_flags +
# absl_flags_parse. Both Debian trixie (libabsl-dev ≥ 20230802) and Fedora 43+
# (abseil-cpp-devel) ship the upstream CMake config files; pkg-config files
# also exist but the CONFIG form is what upstream documents.
find_package(absl CONFIG REQUIRED)
add_library(absl_bundle INTERFACE)
target_link_libraries(absl_bundle INTERFACE absl::flags absl::flags_parse)

# Plugin + compiler binaries. Same path on Debian and Fedora (/usr/bin).
find_program(GRPC_CPP_PLUGIN_PATH grpc_cpp_plugin REQUIRED)
find_program(PROTOC_PATH          protoc          REQUIRED)

message(STATUS "  grpc++:                       ${GRPCPP_VERSION}")
message(STATUS "  protobuf:                     ${PROTOBUF_VERSION}")
message(STATUS "  grpc_cpp_plugin:              ${GRPC_CPP_PLUGIN_PATH}")
message(STATUS "  protoc:                       ${PROTOC_PATH}")

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
# skipped, and HAVE_GLES_CSC stays off. Tests + UAPI libs still build.
pkg_check_modules(GBM IMPORTED_TARGET gbm)
set(HAVE_GBM FALSE)
if(TARGET PkgConfig::GBM)
    set(HAVE_GBM TRUE)
endif()

# librga is Rockchip-only. HAVE_RGA is TRUE iff librga is on the system.
find_library(LIBRGA_LIB rga)
set(HAVE_RGA FALSE)
if(LIBRGA_LIB)
    set(HAVE_RGA TRUE)
    add_library(rga_iface INTERFACE)
    target_link_libraries(rga_iface INTERFACE ${LIBRGA_LIB})
endif()

# HAVE_GLES_CSC = the Mesa MRT-NV12 shader backend. Tracks HAVE_GBM (we
# need GBM-platform EGL to render into dma-buf-backed render targets).
# The backend implementation lands in Phase 2; for now the flag is wired
# so src/CMakeLists.txt can switch on it once the source files arrive.
set(HAVE_GLES_CSC ${HAVE_GBM})

# librockchip_mpp is Rockchip-only. HAVE_MPP is real: TRUE iff librockchip_mpp
# is on the system. When FALSE, mpp_jpeg_dec is skipped and any future
# consumer of mpp_iface should be gated on HAVE_MPP.
find_library(LIBMPP_LIB rockchip_mpp)
set(HAVE_MPP FALSE)
if(LIBMPP_LIB)
    set(HAVE_MPP TRUE)
    add_library(mpp_iface INTERFACE)
    target_link_libraries(mpp_iface INTERFACE ${LIBMPP_LIB})
endif()

# Shared interface bundle for any target that wants EGL/GLES/GBM/DRM in one
# go. Optional — only created when HAVE_GBM.
if(HAVE_GBM)
    add_library(gles_bundle INTERFACE)
    target_link_libraries(gles_bundle INTERFACE
        PkgConfig::EGL PkgConfig::GLESV2 PkgConfig::GBM PkgConfig::DRM)
endif()

# libplacebo — optional, used for evaluation probes (#9). Fedora:
# libplacebo-devel, Debian: libplacebo-dev. When present, builds
# placebo-csc-probe / placebo-vk-probe / placebo-host-copy-probe.
pkg_check_modules(PLACEBO IMPORTED_TARGET libplacebo)
set(HAVE_PLACEBO FALSE)
if(TARGET PkgConfig::PLACEBO)
    set(HAVE_PLACEBO TRUE)
    add_library(placebo_bundle INTERFACE)
    target_link_libraries(placebo_bundle INTERFACE PkgConfig::PLACEBO)
endif()

# Vulkan loader — optional, needed by placebo-vk-probe and
# placebo-host-copy-probe. Usually present if vulkan-loader-devel is
# installed.
pkg_check_modules(VULKAN IMPORTED_TARGET vulkan)
set(HAVE_VULKAN FALSE)
if(TARGET PkgConfig::VULKAN)
    set(HAVE_VULKAN TRUE)
endif()

message(STATUS "videonode-native deps:")
message(STATUS "  GBM (libgbm-dev):             ${HAVE_GBM}")
message(STATUS "  libturbojpeg (libjpeg-turbo): ${TURBOJPEG_VERSION}")
message(STATUS "  librga (Rockchip):            ${HAVE_RGA}")
message(STATUS "  GLES MRT CSC backend:         ${HAVE_GLES_CSC}")
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
if(NOT HAVE_RGA AND NOT HAVE_GLES_CSC)
    message(WARNING
        "  No CSC backend available (HAVE_RGA=off, HAVE_GLES_CSC=off). "
        "videonode-source will still build, but non-NV12 V4L2 sources will "
        "be dropped at runtime. Install librga (RK3588) or libgbm-dev "
        "(generic Linux) to enable a real backend.")
endif()

# GoogleTest — fetched + built in-tree so the dev box doesn't need a system
# install and every CI lane sees the same version. SHA-pinned to v1.15.2
# (release-1.15.2 commit on google/googletest). Bump deliberately, not by
# tag-following.
if(BUILD_TESTS)
    include(FetchContent)
    FetchContent_Declare(googletest
        GIT_REPOSITORY https://github.com/google/googletest.git
        GIT_TAG b514bdc898e2951020cbdca1304b75f5950d1f59) # v1.15.2
    set(INSTALL_GTEST OFF CACHE BOOL "" FORCE)
    set(gtest_force_shared_crt ON CACHE BOOL "" FORCE)
    FetchContent_MakeAvailable(googletest)
    include(GoogleTest)
endif()
