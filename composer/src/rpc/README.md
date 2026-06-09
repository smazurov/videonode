# rpc/

gRPC server lifecycle wrapper (`grpc_server`) plus the strong-typed
composer request structs (`composer_rpc`, header-only) that the daemon→
composer control surface marshals proto messages into. The wire format is
gRPC/protobuf, not JSON-RPC; schemas live in `proto/control/*.proto`.

ctest label: none — the `composer_rpc` structs are exercised via the
`render` World unit tests; `grpc_server` is covered end-to-end.

Invariant: `composer_rpc::Request` structs are the stable `World::apply_*`
boundary — keep proto details from leaking past the service handler so the
World unit-test surface stays pure C++.
