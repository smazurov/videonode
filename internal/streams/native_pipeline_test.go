//go:build planv2_tests

// Tests for the native-binary availability check post-rewrite. The
// post-B9 native pipeline still spawns videonode-source / vn-sink /
// videonode-composer the same way — the test surface is unchanged
// other than the rename Composer → ComposerBin (consistent with the
// three-binary set). This file gets re-enabled when B9 lands.
package streams

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNativePipeline_AvailabilityDetectsExecutables(t *testing.T) {
	dir := t.TempDir()
	exePath := filepath.Join(dir, "fake-bin")
	if err := os.WriteFile(exePath, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	nonExe := filepath.Join(dir, "not-exe")
	if err := os.WriteFile(nonExe, []byte("just data"), 0o644); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(dir, "does-not-exist")

	n := (&NativePipelineConfig{
		V4L2Source: exePath,
		VNSink:     nonExe,
		Composer:   missing,
	}).Resolve(nil)

	if !n.Available.V4L2Source {
		t.Error("expected V4L2Source available for executable")
	}
	if n.Available.VNSink {
		t.Error("expected VNSink NOT available for non-exec file")
	}
	if n.Available.Composer {
		t.Error("expected Composer NOT available for missing file")
	}
}

func TestNativePipeline_AllThreeBinariesNeededForCanvasStream(t *testing.T) {
	// Post-rewrite, every stream of N>1 inputs OR with effects requires
	// the composer. The old SingleStreamReady/CanvasReady helpers stay,
	// but their semantics now read: "single stream" = source+sink up;
	// "canvas" (= composer-engaged stream) = all three up.
	cases := []struct {
		name             string
		v4l2, sink, comp bool
		wantSingle       bool
		wantCanvas       bool
	}{
		{"all present", true, true, true, true, true},
		{"only source", true, false, false, false, false},
		{"source + sink", true, true, false, true, false},
		{"source + composer", true, false, true, false, true},
		{"nothing", false, false, false, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			n := &NativePipelineConfig{
				Available: NativeAvailability{
					V4L2Source: tc.v4l2,
					VNSink:     tc.sink,
					Composer:   tc.comp,
				},
			}
			if got := n.SingleStreamReady(); got != tc.wantSingle {
				t.Errorf("SingleStreamReady: got %v want %v", got, tc.wantSingle)
			}
			if got := n.CanvasReady(); got != tc.wantCanvas {
				t.Errorf("CanvasReady: got %v want %v", got, tc.wantCanvas)
			}
		})
	}
}

func TestNativePipeline_NilReceiverSafe(t *testing.T) {
	var n *NativePipelineConfig
	if n.SingleStreamReady() {
		t.Error("nil receiver should not report ready")
	}
	if n.CanvasReady() {
		t.Error("nil receiver should not report ready")
	}
}

func TestNativePipeline_ExpandsHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no $HOME")
	}
	exePath := filepath.Join(home, ".local/bin/should-not-exist-vn-test")
	n := (&NativePipelineConfig{V4L2Source: "~/.local/bin/should-not-exist-vn-test"}).Resolve(nil)
	if n.V4L2Source != exePath {
		t.Errorf("tilde not expanded: got %q want %q", n.V4L2Source, exePath)
	}
}
