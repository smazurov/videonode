# rpc/

JSON-RPC 2.0 framing + dma-buf metadata envelopes + bidirectional control
channel between Go daemon and C++ sidecars.

ctest label: `rpc`

Invariant: every decoder rejects truncated/malformed input and never
reads past the bytes it was given (fuzz-target shape). No allocations
outside the result struct on the error path.
