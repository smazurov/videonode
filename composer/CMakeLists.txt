cmake_minimum_required(VERSION 3.20)

# C is enabled for the host-side Rockchip stubs (third_party/rockchip-stubs/
# stubs.c). On the rig nothing C is compiled.
project(videonode-native
    LANGUAGES C CXX
    VERSION 0.1.0
    DESCRIPTION "Native dma-buf video pipeline for RK3588")

# ───────────────────────────────────────────────── top-level options
set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_CXX_EXTENSIONS OFF)
set(CMAKE_EXPORT_COMPILE_COMMANDS ON)

if(NOT CMAKE_BUILD_TYPE)
    set(CMAKE_BUILD_TYPE Release CACHE STRING "Build type" FORCE)
endif()

add_compile_options(-Wall -Wextra)

option(BUILD_TESTS "Build host-side unit tests under tests/" ON)

# Default to a per-user install prefix so `cmake --install build` doesn't
# require sudo. Override with -DCMAKE_INSTALL_PREFIX=/usr (or anywhere
# else) for a system-wide install.
if(CMAKE_INSTALL_PREFIX_INITIALIZED_TO_DEFAULT)
    set(CMAKE_INSTALL_PREFIX "$ENV{HOME}/.local"
        CACHE PATH "Install location (default: per-user $HOME/.local)" FORCE)
endif()

# All inter-target #include "src/foo.hpp" / "src/foo.h" paths assume the
# repo root is on the include path.
include_directories(${CMAKE_SOURCE_DIR})

# Generated single-source-of-truth version header. Every binary that wants
# --version pulls in <version.hpp> from the build tree.
configure_file(
    ${CMAKE_SOURCE_DIR}/src/version.hpp.in
    ${CMAKE_BINARY_DIR}/generated/version.hpp
    @ONLY)
include_directories(${CMAKE_BINARY_DIR}/generated)

list(APPEND CMAKE_MODULE_PATH "${CMAKE_SOURCE_DIR}/cmake")
include(vn)
include(Dependencies)
include(Lint)

# ───────────────────────────────────────────────── subdirectories
add_subdirectory(src)
add_subdirectory(tools)
if(BUILD_TESTS)
    enable_testing()
    add_subdirectory(tests)
endif()

# ───────────────────────────────────────────────── packaging
# Builds deb + rpm when dpkg-deb / rpmbuild are available. Always builds
# a .tar.gz. Run `cpack` after `cmake --build`.
include(Packaging)

message(STATUS "")
message(STATUS "videonode-native ${PROJECT_VERSION} configured.")
message(STATUS "  install prefix:   ${CMAKE_INSTALL_PREFIX}")
message(STATUS "  build type:       ${CMAKE_BUILD_TYPE}")
message(STATUS "  tests:            ${BUILD_TESTS}")
message(STATUS "")
message(STATUS "Common commands:")
message(STATUS "  cmake --build ${CMAKE_BINARY_DIR}")
message(STATUS "  ctest --test-dir ${CMAKE_BINARY_DIR}")
message(STATUS "  cmake --install ${CMAKE_BINARY_DIR}            # to ${CMAKE_INSTALL_PREFIX}")
message(STATUS "  cmake --build ${CMAKE_BINARY_DIR} --target lint    # clang-format check")
message(STATUS "  cmake --build ${CMAKE_BINARY_DIR} --target format  # clang-format rewrite")
message(STATUS "  cpack --config ${CMAKE_BINARY_DIR}/CPackConfig.cmake  # deb/rpm/tgz")
message(STATUS "")
