# Packaging.cmake — CPack rules for deb / rpm.
#
# Asymmetric install layout (per the ship plan):
#   - RPM (Fedora): relocatable; install command is
#       rpm -ivh --prefix=$HOME/.local videonode-native-*.rpm
#     so binaries land in ~/.local/bin where the Go daemon expects them
#     (NativeV4L2Source/Sink/Composer defaults). Override --prefix for a
#     system install.
#   - DEB (Armbian/Ubuntu): system /usr/bin. DEB has no relocation
#     equivalent (no `dpkg --prefix`, no CPACK_DEB_RELOCATABLE), so the
#     install lands in /usr/bin and the rig's videonode config is
#     expected to point NATIVE_PIPELINE_* at /usr/bin/videonode-*.
#
# Usage:
#   cmake -B build -S composer -G Ninja -DCMAKE_BUILD_TYPE=Release
#   cmake --build build
#   cpack -G RPM --config build/CPackConfig.cmake   # on Fedora
#   cpack -G DEB --config build/CPackConfig.cmake   # on the rig (or arm64 container)

set(CPACK_PACKAGE_NAME              "videonode-native")
set(CPACK_PACKAGE_VENDOR            "Stepan Mazurov")
set(CPACK_PACKAGE_DESCRIPTION_SUMMARY
    "Native dma-buf video pipeline — videonode-source, videonode-sink, videonode-composer")
set(CPACK_PACKAGE_VERSION_MAJOR     "${PROJECT_VERSION_MAJOR}")
set(CPACK_PACKAGE_VERSION_MINOR     "${PROJECT_VERSION_MINOR}")
set(CPACK_PACKAGE_VERSION_PATCH     "${PROJECT_VERSION_PATCH}")
if(EXISTS "${CMAKE_SOURCE_DIR}/LICENSE")
    set(CPACK_RESOURCE_FILE_LICENSE "${CMAKE_SOURCE_DIR}/LICENSE")
endif()
set(CPACK_RESOURCE_FILE_README "${CMAKE_SOURCE_DIR}/README.md")
set(CPACK_PACKAGING_INSTALL_PREFIX  "/usr")

# Tag every artifact with arch + distro so a Fedora rpm and a rig deb can
# sit next to each other in one /artifacts/ directory.
if(CMAKE_SYSTEM_PROCESSOR MATCHES "^(aarch64|arm64)$")
    set(_vn_arch "arm64")
elseif(CMAKE_SYSTEM_PROCESSOR MATCHES "^(x86_64|amd64)$")
    set(_vn_arch "x86_64")
else()
    set(_vn_arch "${CMAKE_SYSTEM_PROCESSOR}")
endif()
set(CPACK_PACKAGE_FILE_NAME
    "${CPACK_PACKAGE_NAME}-${PROJECT_VERSION}-${_vn_arch}")

# Generate per-target packagers when their tooling is on PATH.
set(CPACK_GENERATOR "TGZ") # always produces a tarball
find_program(DPKG_DEB dpkg-deb)
if(DPKG_DEB)
    list(APPEND CPACK_GENERATOR "DEB")
endif()
find_program(RPMBUILD rpmbuild)
if(RPMBUILD)
    list(APPEND CPACK_GENERATOR "RPM")
endif()

# ── DEB ────────────────────────────────────────────────────────────────
# Lands in /usr/bin. Runtime deps best-effort for current Armbian /
# Ubuntu 22.04 / 24.04; tighten on the rig if dpkg complains. Rockchip
# RGA / MPP are vendor-shipped under varying names on RK3588 distros and
# stay unlisted.
set(CPACK_DEBIAN_PACKAGE_MAINTAINER "smazurov@gmail.com")
set(CPACK_DEBIAN_PACKAGE_SECTION    "video")
set(CPACK_DEBIAN_PACKAGE_DEPENDS
    "libc6, libstdc++6, libegl1, libgles2, libgbm1, libdrm2, ffmpeg")
# DEB file naming: lower-case + underscores + Debian's arch convention
# (amd64 not x86_64, arm64 stays arm64).
if(_vn_arch STREQUAL "x86_64")
    set(_deb_arch "amd64")
else()
    set(_deb_arch "${_vn_arch}")
endif()
string(TOLOWER "${CPACK_PACKAGE_NAME}_${PROJECT_VERSION}_${_deb_arch}.deb"
    CPACK_DEBIAN_FILE_NAME)

# ── RPM ────────────────────────────────────────────────────────────────
# Relocatable; default prefix in the headers stays /usr (FHS), users
# override at install time with `rpm --prefix=$HOME/.local`.
set(CPACK_RPM_PACKAGE_LICENSE       "AGPL-3.0-or-later")
set(CPACK_RPM_PACKAGE_REQUIRES      "mesa-libEGL, mesa-libGLES, mesa-libgbm, libdrm, ffmpeg")
set(CPACK_RPM_PACKAGE_GROUP         "Applications/Multimedia")
set(CPACK_RPM_PACKAGE_RELOCATABLE   TRUE)
# CPACK_RPM_PACKAGE_AUTOREQ stays on (default) — pulls in the actual
# soname deps from the binaries. Add hard Requires above so the package
# still installs cleanly on a minimal Fedora.
set(CPACK_RPM_FILE_NAME "RPM-DEFAULT")

include(CPack)
