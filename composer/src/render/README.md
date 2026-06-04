# render/

GPU compose + colorspace conversion + NV12 allocator. CSC dispatcher
picks RGA on rig (HAVE_RGA) or GLES2 two-pass MRT on host (HAVE_GLES_CSC).

ctest label: `render`

Invariant: NV12 output buffers are always 64-byte-aligned (RGA
requirement) and use the single-bo dma_heap path on rig vs two-bo GBM
split on radeonsi. Never `gbm_bo_map` for a long-lived mapping —
radeonsi treats it as single-shot snapshot.
