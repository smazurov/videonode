//go:build smoke

package smoke

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestPipeline(t *testing.T) {
	requireFfprobe(t)

	const id = "smoke-pipeline"

	defer func() {
		if t.Failed() {
			dumpServerLogTail(t, 200)
		}
	}()

	rtspURL := fmt.Sprintf("rtsp://127.0.0.1:%d/%s", rtspPort, id)
	srtURL := fmt.Sprintf("srt://127.0.0.1:%d?streamid=%s", srtPort, id)

	// The bootstrap stream's "running" state isn't reliably observable from
	// our smoke harness — we connect to /api/streams after the server has
	// already emitted its state-change event, and the Enabled flag on the
	// API model is tied to device readiness (not test_mode liveness). So we
	// drive the assertion straight from ffprobe with a retry loop: if the
	// pipeline is producing frames within the deadline, codec_name will
	// come back as "h264".
	codec, err := retryProbeCodec(t, 25*time.Second, rtspURL, "-rtsp_transport", "tcp")
	if err != nil {
		t.Fatalf("ffprobe RTSP %s: %v", rtspURL, err)
	}
	if codec != "h264" {
		t.Fatalf("ffprobe RTSP %s: codec=%q, want h264", rtspURL, codec)
	}
	t.Logf("RTSP probe OK: %s -> codec=%s", rtspURL, codec)

	// SRT consumer skips frames until the first H264 IDR (keyframe). The
	// pipeline runs at fps=30 with keyframe interval -g 60, so a new
	// subscriber waits up to 2s for the next IDR. Give ffprobe enough
	// patience to receive one full GOP and detect MPEG-TS.
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

// retryProbeCodec calls probeCodec in a loop until success or deadline.
// Each attempt gets 5s; the overall deadline bounds total retries.
func retryProbeCodec(t *testing.T, total time.Duration, url string, extraArgs ...string) (string, error) {
	t.Helper()
	return retryProbeCodecLong(t, total, 5*time.Second, url, extraArgs...)
}

// retryProbeCodecLong is like retryProbeCodec but with a configurable
// per-attempt timeout — useful for SRT which needs to wait for a keyframe.
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
