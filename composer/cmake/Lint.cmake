# Lint.cmake — `make lint` / `make format` targets driven by clang-format.
# `lint` runs in dry-run mode and fails the build if any file would be
# rewritten; `format` rewrites in place. Both targets are no-ops when
# clang-format isn't installed.

find_program(CLANG_FORMAT clang-format)

# Discover sources to lint: every .cpp/.hpp/.h under src/, tools/, tests/.
# CONFIGURE_DEPENDS so cmake re-globs when files appear/disappear.
file(GLOB_RECURSE _vn_lint_sources CONFIGURE_DEPENDS
    "${CMAKE_SOURCE_DIR}/src/*.cpp"
    "${CMAKE_SOURCE_DIR}/src/*.hpp"
    "${CMAKE_SOURCE_DIR}/src/*.h"
    "${CMAKE_SOURCE_DIR}/tools/*.cpp"
    "${CMAKE_SOURCE_DIR}/tools/*.hpp"
    "${CMAKE_SOURCE_DIR}/tests/*.cpp"
    "${CMAKE_SOURCE_DIR}/tests/*.hpp")
list(FILTER _vn_lint_sources EXCLUDE REGEX "/build[^/]*/")

if(CLANG_FORMAT)
    add_custom_target(format
        COMMAND ${CLANG_FORMAT} -i --style=file ${_vn_lint_sources}
        WORKING_DIRECTORY ${CMAKE_SOURCE_DIR}
        COMMENT "clang-format -i on ${list_length} files"
        VERBATIM)

    add_custom_target(lint
        COMMAND ${CLANG_FORMAT} --dry-run --Werror --style=file ${_vn_lint_sources}
        WORKING_DIRECTORY ${CMAKE_SOURCE_DIR}
        COMMENT "clang-format dry-run (fails on diffs)"
        VERBATIM)
else()
    add_custom_target(format
        COMMAND ${CMAKE_COMMAND} -E echo
            "clang-format not found — install it to enable `make format`"
        VERBATIM)
    add_custom_target(lint
        COMMAND ${CMAKE_COMMAND} -E echo
            "clang-format not found — install it to enable `make lint`"
        VERBATIM)
endif()

# Optional clang-tidy integration. Enable with -DENABLE_CLANG_TIDY=ON.
option(ENABLE_CLANG_TIDY "Run clang-tidy on every TU during build" OFF)
if(ENABLE_CLANG_TIDY)
    find_program(CLANG_TIDY clang-tidy)
    if(CLANG_TIDY)
        set(CMAKE_CXX_CLANG_TIDY "${CLANG_TIDY};--quiet")
    else()
        message(WARNING "ENABLE_CLANG_TIDY=ON but clang-tidy not found in PATH")
    endif()
endif()
