//go:build smoke && planv2_tests

// Smoke pipeline test against the post-rewrite TOML shape. Drives a
// test-mode [[sources]] entry through a stream and ffprobes RTSP + SRT
// to confirm frames flow end-to-end.
//
// Awaits B2's v2 TOML loader and B11's validate-config understanding
// the new shape. Once foundation lands, integrators flip the
// planv2_tests build tag and this replaces the v1 smoke pipeline test.
package smoke

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestPipelineV2(t *testing.T) {
	requireFfprobe(t)

	const id = "smoke-pipeline"

	defer func() {
		if t.Failed() {
			dumpServerLogTail(t, 200)
		}
	}()

	rtspURL := fmt.Sprintf("rtsp://127.0.0.1:%d/%s", rtspPort, id)
	srtURL := fmt.Sprintf("srt://127.0.0.1:%d?streamid=%s", srtPort, id)

	codec, err := retryProbeCodec(t, 25*time.Second, rtspURL, "-rtsp_transport", "tcp")
	if err != nil {
		t.Fatalf("ffprobe RTSP %s: %v", rtspURL, err)
	}
	if codec != "h264" {
		t.Fatalf("ffprobe RTSP %s: codec=%q, want h264", rtspURL, codec)
	}
	t.Logf("RTSP probe OK: %s -> codec=%s", rtspURL, codec)

	codec, err = retryProbeCodecLong(t, 40*time.Second, 15*time.Second, srtURL,
		"-analyzeduration", "5000000",
		"-probesize", "5000000")
	if err != nil {
		t.Fatalf("ffprobe SRT %s: %v", srtURL, err)
	}
	if codec != "h264" {
		t.Fatalf("ffprobe SRT %s: codec=%q, want h264", srtURL, codec)
	}
	t.Logf("SRT probe OK: %s -> codec=%s", srtURL, codec)
}

// The v2-shape fixture this test runs against lives at
// testdata/streams.smoke.v2.toml. The smoke harness (main_test.go)
// reads that file post-foundation in place of the v1 fixture.

func retryProbeCodec(t *testing.T, total time.Duration, url string, extraArgs ...string) (string, error) {
	t.Helper()
	return retryProbeCodecLong(t, total, 5*time.Second, url, extraArgs...)
}

func retryProbeCodecLong(t *testing.T, total, perAttempt time.Duration, url string, extraArgs ...string) (string, error) {
	t.Helper()
	deadline := time.Now().Add(total)
	var lastErr error
	attempt := 0
	for time.Now().Before(deadline) {
		attempt++
		probeCtx, cancel := context.WithTimeout(context.Background(), perAttempt)
		codec, err := probeCodec(t, probeCtx, url, extraArgs...)
		cancel()
		if err == nil && codec != "" {
			if attempt > 1 {
				t.Logf("probe succeeded on attempt %d", attempt)
			}
			return codec, nil
		}
		lastErr = err
		time.Sleep(1 * time.Second)
	}
	if lastErr == nil {
		lastErr = errors.New("empty codec_name")
	}
	return "", fmt.Errorf("after %d attempts: %w", attempt, lastErr)
}
