cmake_minimum_required(VERSION 3.30)

project(videonode-native
    LANGUAGES CXX
    VERSION 0.1.0
    DESCRIPTION "Native dma-buf video pipeline for RK3588")

# ───────────────────────────────────────────────── top-level options
set(CMAKE_CXX_STANDARD 20)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_CXX_EXTENSIONS OFF)
set(CMAKE_EXPORT_COMPILE_COMMANDS ON)

# We do not use C++20 modules (see composer/README.md → Follow-ups). Turning
# off module dependency scanning keeps clang-tidy happy: cmake otherwise
# injects -fdeps-format=p1689r5 / -fmodule-mapper into compile_commands.json,
# which clang-tidy rejects as unknown arguments.
set(CMAKE_CXX_SCAN_FOR_MODULES OFF)

# Mirror the build-tree compile_commands.json to the source tree so tools
# that don't read .clangd (some IDEs, MCP servers, scripts) find it at the
# expected root location.
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

# Build-time version header. Runs git-describe on every build; falls back
# to PROJECT_VERSION when git is unavailable. Writes version.hpp only when
# the content changes to avoid needless recompilation.
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
