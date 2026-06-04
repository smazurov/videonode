# src/sensor/CMakeLists.txt — videonode-sensor runtime.
#
# Dials an analysis composer's NV12 canvas SCM bus, runs an out-of-process
# detector child over a duplex socketpair, and streams normalized Findings to
# the daemon via the Sensor gRPC service. Perception-only: no actuation.

vn_add_library(sensor_runtime
    SOURCES orchestrator.cpp sensor_service.cpp detector_backend.cpp
    PUBLIC_DEPS scm_rights_source grpc_server vn_proto Threads::Threads)
