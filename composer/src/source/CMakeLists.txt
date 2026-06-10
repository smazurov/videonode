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
    PUBLIC_DEPS source_probe vn_proto vn_snapshot)

# y4m_reader: incremental yuv4mpegpipe parser feeding pipe-mode sources.
vn_add_library(y4m_reader
    SOURCES y4m_reader.cpp
    UNIT_SAFE)

# pipe_session: --pipe-cmd child lifecycle + y4m -> NV12 dma-buf ring.
vn_add_library(pipe_session
    SOURCES pipe_session.cpp
    PUBLIC_DEPS nv12_buf y4m_reader
    PRIVATE_DEPS child_process)

vn_add_library(orchestrator
    SOURCES orchestrator.cpp orchestrator_flags.cpp capture_session.cpp broadcast.cpp
            placeholder_ring.cpp source_runtime.cpp pipe_orchestrator.cpp
    PUBLIC_DEPS v4l2_capture csc nv12_buf placeholder_painter source_probe signal_activity
                scm_rights_producer dmabuf_header
                jpeg_dec_turbo
                source_service grpc_server vn_proto
                vn_snapshot pipe_session
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
