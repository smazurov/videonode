// Package smoke contains the videonode end-to-end smoke test suite.
//
// All test files in this package are gated behind the "smoke" build tag and
// run via:
//
//	go test -v -count=1 -timeout=180s -tags=smoke ./test/smoke/...
//
// The suite builds the videonode binary, picks ephemeral ports, spawns the
// server with an isolated config in a temp dir, exercises the REST API,
// tails the SSE event stream until a test_mode stream reaches the running
// state, and ffprobes the live RTSP and SRT outputs. ffprobe must be in
// PATH.
package smoke
