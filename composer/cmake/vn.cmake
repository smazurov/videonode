# vn.cmake — helpers for declaring libraries / binaries / probes / tests
# inside videonode-native. Goal: one-line target declarations so adding a
# new file is cheap and visually consistent.
#
# Helpers (all expect the convention CMAKE_SOURCE_DIR/src is on the include
# path; that's set once in the top-level CMakeLists.txt):
#
#   vn_add_library(<name>
#       [SOURCES <src1> <src2> ...]      # defaults to src/${name}.cpp
#       [PRIVATE_DEPS <t> ...]
#       [PUBLIC_DEPS  <t> ...])
#
#   vn_add_executable(<name>
#       SOURCES <main.cpp> ...
#       [DEPS <t> ...]
#       [STUBS]                          # link rockchip stubs when used
#       [NO_INSTALL])                    # default: installed to bin/
#
#   vn_add_probe(<name>
#       [SOURCES <src.cpp>]              # defaults to tools/<name>.cpp
#       [DEPS <t> ...])                  # never installed
#
#   vn_add_test(<short_name>
#       [SOURCES <src.cpp>]              # defaults to tests/test_<short>.cpp
#       [DEPS <t> ...])                  # registers ctest case

include(GNUInstallDirs)

function(vn_add_library name)
    set(opts)
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
endfunction()

function(vn_add_executable name)
    set(opts STUBS NO_INSTALL)
    set(one_value)
    set(multi_value SOURCES DEPS)
    cmake_parse_arguments(ARG "${opts}" "${one_value}" "${multi_value}" ${ARGN})

    if(NOT ARG_SOURCES)
        message(FATAL_ERROR "vn_add_executable(${name}) requires SOURCES")
    endif()

    add_executable(${name} ${ARG_SOURCES})
    if(ARG_DEPS)
        target_link_libraries(${name} PRIVATE ${ARG_DEPS})
    endif()
    if(ARG_STUBS AND USING_ROCKCHIP_STUBS)
        # The rockchip-stub OBJECT lib doesn't propagate transitively through
        # static libs reliably; pull it into each consumer that needs it.
        target_sources(${name} PRIVATE $<TARGET_OBJECTS:rockchip_stubs>)
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
    set(multi_value SOURCES DEPS)
    cmake_parse_arguments(ARG "${opts}" "${one_value}" "${multi_value}" ${ARGN})

    if(NOT BUILD_TESTS)
        return()
    endif()

    if(NOT ARG_SOURCES)
        set(ARG_SOURCES "${CMAKE_CURRENT_SOURCE_DIR}/test_${short_name}.cpp")
    endif()

    set(target "test_${short_name}")
    add_executable(${target} ${ARG_SOURCES})
    if(ARG_DEPS)
        target_link_libraries(${target} PRIVATE ${ARG_DEPS})
    endif()
    add_test(NAME ${short_name} COMMAND ${target})
endfunction()
