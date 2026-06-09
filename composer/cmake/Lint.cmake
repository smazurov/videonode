# clang-format-driven lint/format targets; no-ops when clang-format is absent.
find_program(CLANG_FORMAT clang-format)

# CONFIGURE_DEPENDS so cmake re-globs when files appear/disappear.
file(GLOB_RECURSE _vn_lint_sources CONFIGURE_DEPENDS
    "${CMAKE_SOURCE_DIR}/src/*.cpp"
    "${CMAKE_SOURCE_DIR}/src/*.hpp"
    "${CMAKE_SOURCE_DIR}/src/*.h"
    "${CMAKE_SOURCE_DIR}/tools/*.cpp"
    "${CMAKE_SOURCE_DIR}/tools/*.hpp"
    "${CMAKE_SOURCE_DIR}/tests/*.cpp"
    "${CMAKE_SOURCE_DIR}/tests/*.hpp"
    "${CMAKE_SOURCE_DIR}/fuzz/*.cpp"
    "${CMAKE_SOURCE_DIR}/fuzz/*.hpp")
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

# tidy-diff lints only lines changed vs TIDY_DIFF_BASE; tidy-all runs the whole
# tree. Base is origin/native since composer/ doesn't exist on origin/main.
set(TIDY_DIFF_BASE "origin/native" CACHE STRING
    "Git ref to diff against for tidy-diff (clang-tidy on changed lines).")

find_program(CLANG_TIDY_BIN NAMES clang-tidy)
find_program(CLANG_TIDY_DIFF
    NAMES clang-tidy-diff.py clang-tidy-diff
    PATHS /usr/share/clang /usr/local/share/clang /usr/bin /usr/local/bin
    DOC "clang-tidy-diff.py wrapper (clang-tools-extra)")
find_program(RUN_CLANG_TIDY
    NAMES run-clang-tidy run-clang-tidy.py
    DOC "run-clang-tidy driver (clang-tools-extra)")

if(CLANG_TIDY_DIFF AND CLANG_TIDY_BIN)
    # Run git from the repo root so diff paths are repo-relative
    # (composer/src/foo.cpp). clang-tidy-diff -p2 strips `b/composer/`, leaving
    # `src/foo.cpp` which resolves correctly with cwd=composer. Wrapped in
    # `sh -c` so the shell handles the pipe.
    add_custom_target(tidy-diff
        COMMAND sh -c "git -C '${CMAKE_SOURCE_DIR}/..' diff -U0 ${TIDY_DIFF_BASE}...HEAD -- composer/src composer/tools composer/tests composer/fuzz | '${CLANG_TIDY_DIFF}' -p2 -path '${CMAKE_BINARY_DIR}' -clang-tidy-binary '${CLANG_TIDY_BIN}'"
        WORKING_DIRECTORY ${CMAKE_SOURCE_DIR}
        COMMENT "clang-tidy on lines changed vs ${TIDY_DIFF_BASE}"
        VERBATIM USES_TERMINAL)
else()
    add_custom_target(tidy-diff
        COMMAND ${CMAKE_COMMAND} -E echo
            "clang-tidy-diff.py or clang-tidy not found — install clang-tools-extra"
        VERBATIM)
endif()

if(RUN_CLANG_TIDY AND CLANG_TIDY_BIN)
    add_custom_target(tidy-all
        COMMAND ${RUN_CLANG_TIDY} -p ${CMAKE_BINARY_DIR} -clang-tidy-binary ${CLANG_TIDY_BIN}
            "^${CMAKE_SOURCE_DIR}/(src|tools|tests|fuzz)/.*"
        WORKING_DIRECTORY ${CMAKE_SOURCE_DIR}
        COMMENT "clang-tidy whole tree (slow)"
        VERBATIM USES_TERMINAL)
else()
    add_custom_target(tidy-all
        COMMAND ${CMAKE_COMMAND} -E echo
            "run-clang-tidy or clang-tidy not found — install clang-tools-extra"
        VERBATIM)
endif()
