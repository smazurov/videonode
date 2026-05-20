# Packaging.cmake — CPack rules for deb / rpm. Generates archives that
# install the binaries into ${CPACK_PACKAGING_INSTALL_PREFIX}/bin and
# declare runtime dependencies appropriate to each package format.
#
# Usage:
#   cmake -B build -S .
#   cmake --build build
#   cpack -G DEB --config build/CPackConfig.cmake
#   cpack -G RPM --config build/CPackConfig.cmake
#
# Or both at once: `cpack -G "DEB;RPM" ...`

set(CPACK_PACKAGE_NAME              "videonode-native")
set(CPACK_PACKAGE_VENDOR            "Stepan Mazurov")
set(CPACK_PACKAGE_DESCRIPTION_SUMMARY
    "Native dma-buf video pipeline for RK3588 — videonode-source, vn-sink, videonode-composer")
set(CPACK_PACKAGE_VERSION_MAJOR     "${PROJECT_VERSION_MAJOR}")
set(CPACK_PACKAGE_VERSION_MINOR     "${PROJECT_VERSION_MINOR}")
set(CPACK_PACKAGE_VERSION_PATCH     "${PROJECT_VERSION_PATCH}")
if(EXISTS "${CMAKE_SOURCE_DIR}/LICENSE")
    set(CPACK_RESOURCE_FILE_LICENSE "${CMAKE_SOURCE_DIR}/LICENSE")
endif()
set(CPACK_RESOURCE_FILE_README "${CMAKE_SOURCE_DIR}/README.md")
set(CPACK_PACKAGING_INSTALL_PREFIX  "/usr")

# Generate both .deb and .rpm by default when packagers are present.
set(CPACK_GENERATOR "TGZ") # always produces a tarball
find_program(DPKG_DEB dpkg-deb)
if(DPKG_DEB)
    list(APPEND CPACK_GENERATOR "DEB")
endif()
find_program(RPMBUILD rpmbuild)
if(RPMBUILD)
    list(APPEND CPACK_GENERATOR "RPM")
endif()

# .deb runtime deps. Best-effort list — package on the target distro to
# validate. ffmpeg and libdrm2 are universal; libgbm1 / libegl1 / libgles2
# are mesa userspace; the rockchip libs are vendor-shipped on RK3588
# distros (Armbian / Rockchip Ubuntu) under varying names.
set(CPACK_DEBIAN_PACKAGE_MAINTAINER "smazurov@gmail.com")
set(CPACK_DEBIAN_PACKAGE_SECTION    "video")
set(CPACK_DEBIAN_PACKAGE_DEPENDS
    "libc6, libstdc++6, libegl1, libgles2, libgbm1, libdrm2, ffmpeg")

# .rpm runtime deps. Naming differs from Debian.
set(CPACK_RPM_PACKAGE_LICENSE    "AGPL-3.0-or-later")
set(CPACK_RPM_PACKAGE_REQUIRES   "mesa-libEGL, mesa-libGLES, mesa-libgbm, libdrm, ffmpeg")
set(CPACK_RPM_PACKAGE_GROUP      "Applications/Multimedia")

include(CPack)
