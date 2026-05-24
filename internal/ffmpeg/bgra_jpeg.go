package ffmpeg

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"time"
)

// EncodeBGRAToJPEG converts a tight-packed BGRA frame to JPEG via a
// 5-second ffmpeg subprocess. Used by the daemon's snapshot cache when
// the composer's Snapshot RPC returns canvas bytes.
func EncodeBGRAToJPEG(bgra []byte, width, height int) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-hide_banner",
		"-f", "rawvideo",
		"-pix_fmt", "bgra",
		"-s", fmt.Sprintf("%dx%d", width, height),
		"-i", "pipe:0",
		"-frames:v", "1",
		"-f", "mjpeg",
		"-q:v", "2",
		"pipe:1",
	)
	cmd.Stdin = bytes.NewReader(bgra)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("bgra to jpeg: %w: %s", err, stderr.String())
	}
	return stdout.Bytes(), nil
}
