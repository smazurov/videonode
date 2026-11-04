# src/sink/CMakeLists.txt — videonode-sink internals: NUT muxing, A/V pacing, ALSA capture.

vn_add_library(nut_writer UNIT_SAFE)

vn_add_library(av_timeline
    SOURCES video_pacer.cpp audio_clock.cpp audio_track.cpp audio_health.cpp status_report.cpp
    UNIT_SAFE)

vn_add_library(status_reporter PUBLIC_DEPS Threads::Threads)

vn_add_library(alsa_capture
    PUBLIC_DEPS av_timeline Threads::Threads
    PRIVATE_DEPS PkgConfig::ALSA)

vn_add_library(nv12_pack PUBLIC_DEPS scm_rights_source)
if(CMAKE_SYSTEM_PROCESSOR MATCHES "x86_64|AMD64")
    target_compile_options(nv12_pack PRIVATE -msse4.1)
endif()

vn_add_library(video_link PUBLIC_DEPS scm_rights_source Threads::Threads)

vn_add_library(raw_session PUBLIC_DEPS nv12_pack video_link scm_rights_source)

vn_add_library(nut_session
    PUBLIC_DEPS nut_writer av_timeline alsa_capture nv12_pack video_link status_reporter
                scm_rights_source)
