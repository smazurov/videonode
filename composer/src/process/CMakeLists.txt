# src/process/CMakeLists.txt — child-process spawn/lifecycle + ffmpeg pipe-source.

vn_add_library(child_process SOURCES child_process.cpp)

if(HAVE_GBM)
    vn_add_library(ffmpeg_pipe_source
        SOURCES ffmpeg_pipe_source.cpp
        PUBLIC_DEPS gbm_alloc
        PRIVATE_DEPS dma_heap child_process Threads::Threads)
endif()
