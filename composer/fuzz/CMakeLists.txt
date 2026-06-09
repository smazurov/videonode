# fuzz/ — libFuzzer harnesses. Built only under the `fuzz` preset
# (ENABLE_FUZZING=ON, clang). Each harness pins one untrusted-bytes decoder.
#
# Run a real campaign by invoking a harness directly:
#   cmake --preset fuzz && cmake --build --preset fuzz
#   ./build/fuzz/fuzz/fuzz_dmabuf_header_decode \
#       fuzz/corpus/dmabuf_header_decode -max_total_time=60
#
# The bounded -runs replay vn_add_fuzzer registers as a ctest (LABEL fuzz) is
# only a regression smoke — it replays the committed seeds and a fixed-seed
# mutation budget so a regression in the decoder fails CI deterministically.

vn_add_fuzzer(dmabuf_header_decode
    DEPS dmabuf_header
    CORPUS "${CMAKE_CURRENT_SOURCE_DIR}/corpus/dmabuf_header_decode")
