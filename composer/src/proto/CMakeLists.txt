# Generated proto + gRPC stubs for the daemon ↔ native control plane.
#
# Source-of-truth lives at <repo>/proto/control/*.proto. We invoke protoc
# at configure time to populate ${CMAKE_BINARY_DIR}/generated/control/
# with the *.pb.{cc,h} and *.grpc.pb.{cc,h} files. Consumer C++ code
# includes them as "control/source.pb.h", "control/source.grpc.pb.h", etc.
#
# PROTOC_PATH and GRPC_CPP_PLUGIN_PATH are resolved in
# composer/cmake/Dependencies.cmake — either via the gRPC CMake config
# targets when present, or via find_program() over the system PATH.

set(PROTO_ROOT    "${CMAKE_SOURCE_DIR}/../proto")
set(PROTO_OUT_DIR "${CMAKE_BINARY_DIR}/generated")

# All proto files (cpp output for every one of them).
set(PROTO_FILES
    control/common.proto
    control/source.proto
    control/composer.proto)

# Proto files that contain `service` blocks (gRPC stubs only for these).
set(PROTO_SERVICE_FILES
    control/source.proto
    control/composer.proto)

# Expected outputs — listed explicitly so CMake regenerates when any
# .proto changes and so the OBJECT library below picks up real paths.
set(PROTO_GEN_SRCS
    ${PROTO_OUT_DIR}/control/common.pb.cc
    ${PROTO_OUT_DIR}/control/source.pb.cc
    ${PROTO_OUT_DIR}/control/composer.pb.cc)
set(PROTO_GEN_HDRS
    ${PROTO_OUT_DIR}/control/common.pb.h
    ${PROTO_OUT_DIR}/control/source.pb.h
    ${PROTO_OUT_DIR}/control/composer.pb.h)
set(GRPC_GEN_SRCS
    ${PROTO_OUT_DIR}/control/source.grpc.pb.cc
    ${PROTO_OUT_DIR}/control/composer.grpc.pb.cc)
set(GRPC_GEN_HDRS
    ${PROTO_OUT_DIR}/control/source.grpc.pb.h
    ${PROTO_OUT_DIR}/control/composer.grpc.pb.h)

# Resolve absolute paths for DEPENDS so the rebuild rule is correct.
set(PROTO_ABS_FILES "")
foreach(_p IN LISTS PROTO_FILES)
    list(APPEND PROTO_ABS_FILES "${PROTO_ROOT}/${_p}")
endforeach()

file(MAKE_DIRECTORY "${PROTO_OUT_DIR}/control")

add_custom_command(
    OUTPUT  ${PROTO_GEN_SRCS} ${PROTO_GEN_HDRS} ${GRPC_GEN_SRCS} ${GRPC_GEN_HDRS}
    COMMAND ${PROTOC_PATH}
            -I ${PROTO_ROOT}
            --cpp_out=${PROTO_OUT_DIR}
            ${PROTO_FILES}
    COMMAND ${PROTOC_PATH}
            -I ${PROTO_ROOT}
            --grpc_out=${PROTO_OUT_DIR}
            --plugin=protoc-gen-grpc=${GRPC_CPP_PLUGIN_PATH}
            ${PROTO_SERVICE_FILES}
    DEPENDS ${PROTO_ABS_FILES}
    WORKING_DIRECTORY ${PROTO_ROOT}
    COMMENT "Regenerating C++ proto + gRPC stubs into ${PROTO_OUT_DIR}/control/"
    VERBATIM)

# OBJECT library — every binary that needs the generated types links it
# transitively and we avoid duplicate compilation. PUBLIC linkage on the
# include path so consumers can `#include "control/source.pb.h"`.
add_library(vn_proto OBJECT
    ${PROTO_GEN_SRCS}
    ${GRPC_GEN_SRCS})
target_include_directories(vn_proto PUBLIC ${PROTO_OUT_DIR})
target_link_libraries(vn_proto PUBLIC grpc_bundle)

# Generated code triggers warnings we can't fix at our end; relax them
# locally so the rest of the tree keeps `-Wall -Wextra -Wshadow` strict.
target_compile_options(vn_proto PRIVATE -w)
