# Floor is bookworm's cmake (the bare-configure release target); presets need 3.30.
cmake_minimum_required(VERSION 3.25)

project(videonode-native
    LANGUAGES CXX
    VERSION 0.1.0
    DESCRIPTION "Native dma-buf video pipeline for RK3588")

# ───────────────────────────────────────────────── top-level options
set(CMAKE_CXX_STANDARD 20)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_CXX_EXTENSIONS OFF)
set(CMAKE_EXPORT_COMPILE_COMMANDS ON)

# No C++20 modules; scanning injects flags clang-tidy rejects.
set(CMAKE_CXX_SCAN_FOR_MODULES OFF)

# Mirror compile_commands.json to the source tree for tools that don't read .clangd.
if(CMAKE_EXPORT_COMPILE_COMMANDS)
    add_custom_target(vn_compile_commands_symlink ALL
        COMMAND ${CMAKE_COMMAND} -E create_symlink
            ${CMAKE_BINARY_DIR}/compile_commands.json
            ${CMAKE_SOURCE_DIR}/compile_commands.json
        BYPRODUCTS ${CMAKE_SOURCE_DIR}/compile_commands.json
        COMMENT "Symlinking compile_commands.json into source tree"
        VERBATIM)
endif()

if(NOT CMAKE_BUILD_TYPE)
    set(CMAKE_BUILD_TYPE Release CACHE STRING "Build type" FORCE)
endif()

add_compile_options(-Wall -Wextra -Wshadow)

# Opt-in: OFF keeps gtest off the shipping/release path; dev/sanitizer/lint
# presets set it ON to build and run the suite.
option(BUILD_TESTS "Build host-side unit tests under tests/" OFF)

# Per-user install prefix so `cmake --install` needs no sudo; override with
# -DCMAKE_INSTALL_PREFIX=/usr for a system-wide install.
if(CMAKE_INSTALL_PREFIX_INITIALIZED_TO_DEFAULT)
    set(CMAKE_INSTALL_PREFIX "$ENV{HOME}/.local"
        CACHE PATH "Install location (default: per-user $HOME/.local)" FORCE)
endif()

# Inter-target #include "src/foo.hpp" paths assume repo root on the include path.
include_directories(${CMAKE_SOURCE_DIR})

# Build-time version header from git-describe (falls back to PROJECT_VERSION).
add_custom_target(vn_version ALL
    COMMAND ${CMAKE_COMMAND}
        -DSOURCE_DIR=${CMAKE_SOURCE_DIR}
        -DBINARY_DIR=${CMAKE_BINARY_DIR}
        -DPROJECT_NAME=${PROJECT_NAME}
        -P ${CMAKE_SOURCE_DIR}/cmake/GenerateVersion.cmake
    BYPRODUCTS ${CMAKE_BINARY_DIR}/generated/version.hpp
    COMMENT "Stamping version.hpp"
    VERBATIM)
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
if(ENABLE_FUZZING)
    enable_testing()
    add_subdirectory(fuzz)
endif()


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
message(STATUS "")
