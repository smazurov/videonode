# vn.cmake — one-line target helpers for videonode-native. Assumes
# CMAKE_SOURCE_DIR/src is on the include path (set in the top-level CMakeLists).
#
#   vn_add_library(<name>    [SOURCES ...] [PRIVATE_DEPS ...] [PUBLIC_DEPS ...] [UNIT_SAFE])
#       SOURCES defaults to src/<name>.cpp
#       UNIT_SAFE appends <name> to the global VN_UNIT_SAFE_LIBS property so
#       vn_add_unit_suite picks it up automatically.
#   vn_add_executable(<name> SOURCES <main.cpp> ... [DEPS ...] [NO_INSTALL])
#       installed to bin/ unless NO_INSTALL
#   vn_add_probe(<name>      [SOURCES ...] [DEPS ...])    never installed
#   vn_add_test(<short_name> [SOURCES ...] [DEPS ...] [LABELS ...])
#       SOURCES defaults to tests/test_<short>.cpp; LABELS forwarded to ctest
#   vn_add_unit_suite(<binary> GLOB <dir> [LABELS ...])
#       Globs test_*.cpp from <dir> (CONFIGURE_DEPENDS), links all UNIT_SAFE
#       libs, and registers each TEST() case as its own ctest entry.

include(GNUInstallDirs)

function(vn_add_library name)
    set(opts UNIT_SAFE)
    set(one_value)
    set(multi_value SOURCES PRIVATE_DEPS PUBLIC_DEPS)
    cmake_parse_arguments(ARG "${opts}" "${one_value}" "${multi_value}" ${ARGN})

    if(NOT ARG_SOURCES)
        set(ARG_SOURCES "${CMAKE_CURRENT_SOURCE_DIR}/${name}.cpp")
    endif()

    add_library(${name} STATIC ${ARG_SOURCES})

    if(ARG_PRIVATE_DEPS)
        target_link_libraries(${name} PRIVATE ${ARG_PRIVATE_DEPS})
    endif()
    if(ARG_PUBLIC_DEPS)
        target_link_libraries(${name} PUBLIC ${ARG_PUBLIC_DEPS})
    endif()
    if(ARG_UNIT_SAFE)
        set_property(GLOBAL APPEND PROPERTY VN_UNIT_SAFE_LIBS ${name})
    endif()
endfunction()

function(vn_add_unit_suite binary)
    set(opts)
    set(one_value GLOB)
    set(multi_value LABELS)
    cmake_parse_arguments(ARG "${opts}" "${one_value}" "${multi_value}" ${ARGN})

    if(NOT BUILD_TESTS)
        return()
    endif()

    file(GLOB _sources CONFIGURE_DEPENDS "${ARG_GLOB}/test_*.cpp")
    get_property(_unit_libs GLOBAL PROPERTY VN_UNIT_SAFE_LIBS)

    add_executable(${binary} ${_sources})
    target_link_libraries(${binary} PRIVATE GTest::gtest_main GTest::gmock ${_unit_libs})
    gtest_discover_tests(${binary} PROPERTIES LABELS "${ARG_LABELS}")
endfunction()

function(vn_add_executable name)
    set(opts NO_INSTALL)
    set(one_value)
    set(multi_value SOURCES DEPS)
    cmake_parse_arguments(ARG "${opts}" "${one_value}" "${multi_value}" ${ARGN})

    if(NOT ARG_SOURCES)
        message(FATAL_ERROR "vn_add_executable(${name}) requires SOURCES")
    endif()

    add_executable(${name} ${ARG_SOURCES})
    add_dependencies(${name} vn_version)
    if(ARG_DEPS)
        target_link_libraries(${name} PRIVATE ${ARG_DEPS})
    endif()
    if(NOT ARG_NO_INSTALL)
        install(TARGETS ${name} RUNTIME DESTINATION ${CMAKE_INSTALL_BINDIR})
    endif()
endfunction()

function(vn_add_probe name)
    set(opts)
    set(one_value)
    set(multi_value SOURCES DEPS)
    cmake_parse_arguments(ARG "${opts}" "${one_value}" "${multi_value}" ${ARGN})

    if(NOT ARG_SOURCES)
        set(ARG_SOURCES "${CMAKE_CURRENT_SOURCE_DIR}/${name}.cpp")
    endif()

    add_executable(${name} ${ARG_SOURCES})
    if(ARG_DEPS)
        target_link_libraries(${name} PRIVATE ${ARG_DEPS})
    endif()
endfunction()

function(vn_add_test short_name)
    set(opts)
    set(one_value)
    set(multi_value SOURCES DEPS LABELS)
    cmake_parse_arguments(ARG "${opts}" "${one_value}" "${multi_value}" ${ARGN})

    if(NOT BUILD_TESTS)
        return()
    endif()

    if(NOT ARG_SOURCES)
        set(ARG_SOURCES "${CMAKE_CURRENT_SOURCE_DIR}/test_${short_name}.cpp")
    endif()

    set(target "test_${short_name}")
    add_executable(${target} ${ARG_SOURCES})
    target_link_libraries(${target} PRIVATE GTest::gtest_main GTest::gmock)
    if(ARG_DEPS)
        target_link_libraries(${target} PRIVATE ${ARG_DEPS})
    endif()

    if(ARG_LABELS)
        gtest_discover_tests(${target}
            PROPERTIES LABELS "${ARG_LABELS}")
    else()
        gtest_discover_tests(${target})
    endif()
endfunction()
