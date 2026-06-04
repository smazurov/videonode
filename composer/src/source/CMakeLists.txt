# src/source/CMakeLists.txt — videonode-source orchestrator library.
#
# Extracted from bin/videonode_source_main.cpp (Phase F.4a). Owns the
# capture-session lifecycle, the broadcast loop, the placeholder ring,
# and the control-plane handler. The bin/ target only wires signals and
# calls source::Run.

# source_service: gRPC service impl for videonode.control.Source.
# Exposes SetFormat / GetStatus / StreamStatus / Snapshot / Shutdown /
# Describe to the daemon. The orchestrator publishes status snapshots
# and latest-frame copies via the producer-thread entry points.
vn_add_library(source_service
    SOURCES source_service.cpp
    PUBLIC_DEPS source_probe vn_proto grpc_server vn_snapshot)

vn_add_library(orchestrator
    SOURCES orchestrator.cpp orchestrator_flags.cpp capture_session.cpp broadcast.cpp
    PUBLIC_DEPS v4l2_capture csc nv12_buf placeholder_painter source_probe signal_activity
                scm_rights_producer dmabuf_header dma_heap
                jpeg_dec_turbo
                source_service grpc_server vn_proto
                vn_snapshot
                absl_bundle
                Threads::Threads)
if(HAVE_RGA)
    target_compile_definitions(orchestrator PRIVATE HAVE_RGA=1)
endif()
if(HAVE_MPP)
    target_link_libraries(orchestrator PUBLIC mpp_jpeg_dec)
    target_compile_definitions(orchestrator PRIVATE HAVE_MPP=1)
endif()
if(HAVE_PLACEBO AND HAVE_GBM AND NOT HAVE_RGA)
    target_compile_definitions(orchestrator PRIVATE HAVE_GBM=1)
    target_link_libraries(orchestrator PUBLIC csc_placebo gles_bundle placebo_bundle)
endif()
