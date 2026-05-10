package ffmpeg

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"time"
)

// EncodeNV12ToJPEG converts a raw NV12 frame to JPEG via a 5-second ffmpeg subprocess.
func EncodeNV12ToJPEG(nv12 []byte, width, height int) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-hide_banner",
		"-f", "rawvideo",
		"-pix_fmt", "nv12",
		"-s", fmt.Sprintf("%dx%d", width, height),
		"-i", "pipe:0",
		"-frames:v", "1",
		"-f", "mjpeg",
		"-q:v", "2",
		"pipe:1",
	)
	cmd.Stdin = bytes.NewReader(nv12)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("nv12 to jpeg: %w: %s", err, stderr.String())
	}
	return stdout.Bytes(), nil
}
