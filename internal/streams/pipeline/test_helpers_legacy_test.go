//go:build !planv2_tests

// Legacy test helpers retained until B1 (Apply* rewrite) lands. The
// post-rewrite pipeline_test.go redefines its own helpers behind the
// planv2_tests tag; meanwhile snapshot_test.go still drives the
// monolithic Stream{Inputs:...} shape and needs these helpers in the
// default build.
package pipeline

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// longRunningBin writes a tiny shell script that ignores its argv and
// sleeps for an hour. Use this as the fake binary path so the pool's
// supervised process stays alive long enough for assertions; t.Cleanup
// drains the pool to kill them at test end.
func longRunningBin(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fake-bin.sh")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexec sleep 3600\n"), 0o755); err != nil {
		t.Fatalf("write fake-bin: %v", err)
	}
	return path
}

func newTestPipeline(t *testing.T, resolveTo string) *Pipeline {
	t.Helper()
	bin := longRunningBin(t)
	p := New(Config{
		VNSourceBin:   bin,
		VNComposerBin: bin,
		VNSinkBin:     bin,
		DRMDevice:     "/dev/dri/renderD128",
		DeviceResolver: func(id string) string {
			if id == "" {
				return ""
			}
			return resolveTo
		},
	}, nil)
	t.Cleanup(func() { p.Pool().StopAll() })
	return p
}

// waitRunning polls IsRunning for up to 1 s, giving the pool's
// state-machine goroutine a chance to mark the process Running.
func waitRunning(p *Pipeline, id string) bool {
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if p.Pool().IsRunning(id) {
			return true
		}
		time.Sleep(2 * time.Millisecond)
	}
	return false
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
