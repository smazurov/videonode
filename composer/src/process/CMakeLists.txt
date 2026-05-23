# src/process/CMakeLists.txt — child-process spawn/lifecycle + ffmpeg pipe-source.

vn_add_library(child_process SOURCES child_process.cpp)
vn_add_library(y4m_writer    SOURCES y4m_writer.cpp)

if(HAVE_GBM)
    vn_add_library(ffmpeg_pipe_source
        SOURCES ffmpeg_pipe_source.cpp
        PUBLIC_DEPS gbm_alloc dma_heap
        PRIVATE_DEPS child_process Threads::Threads)
endif()
