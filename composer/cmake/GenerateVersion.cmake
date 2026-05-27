# GenerateVersion.cmake — invoked at build time via cmake -P.
# Writes generated/version.hpp only when content changes.

set(FALLBACK_VERSION "0.1.0")

find_program(GIT git)
if(GIT)
    execute_process(
        COMMAND ${GIT} describe --tags --always --dirty
        WORKING_DIRECTORY "${SOURCE_DIR}"
        OUTPUT_VARIABLE GIT_VERSION
        ERROR_QUIET
        OUTPUT_STRIP_TRAILING_WHITESPACE
        RESULT_VARIABLE GIT_RESULT)
endif()

if(NOT GIT OR NOT GIT_RESULT EQUAL 0 OR GIT_VERSION STREQUAL "")
    set(GIT_VERSION "${FALLBACK_VERSION}")
endif()

set(VERSION_CONTENT [=[
// version.hpp — generated at build time. Do not edit.
#pragma once

namespace vn {
constexpr const char* kVersion = "VN_GIT_VERSION_PLACEHOLDER";
constexpr const char* kProjectName = "VN_PROJECT_NAME_PLACEHOLDER";
}  // namespace vn
]=])
string(REPLACE "VN_GIT_VERSION_PLACEHOLDER" "${GIT_VERSION}" VERSION_CONTENT "${VERSION_CONTENT}")
string(REPLACE "VN_PROJECT_NAME_PLACEHOLDER" "${PROJECT_NAME}" VERSION_CONTENT "${VERSION_CONTENT}")

set(OUT "${BINARY_DIR}/generated/version.hpp")
if(EXISTS "${OUT}")
    file(READ "${OUT}" EXISTING)
else()
    set(EXISTING "")
endif()
if(NOT EXISTING STREQUAL VERSION_CONTENT)
    file(MAKE_DIRECTORY "${BINARY_DIR}/generated")
    file(WRITE "${OUT}" "${VERSION_CONTENT}")
endif()
