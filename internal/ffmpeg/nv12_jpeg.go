package ffmpeg

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"time"
)

// EncodeNV12ToJPEG converts a raw NV12 frame to JPEG via a 5-second ffmpeg
// subprocess. The colorMatrix arg ("bt601"/"bt709"/"") tags the raw input so
// the YUV→RGB decode uses the right matrix; a raw pipe carries no metadata.
func EncodeNV12ToJPEG(nv12 []byte, width, height int, colorMatrix string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	color := ColorTagsForMatrix(colorMatrix).FFArgs()
	args := make([]string, 0, 14+len(color))
	args = append(args, "-hide_banner", "-f", "rawvideo", "-pix_fmt", "nv12",
		"-s", fmt.Sprintf("%dx%d", width, height))
	args = append(args, color...)
	args = append(args, "-i", "pipe:0", "-frames:v", "1", "-f", "mjpeg", "-q:v", "2", "pipe:1")

	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	cmd.Stdin = bytes.NewReader(nv12)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("nv12 to jpeg: %w: %s", err, stderr.String())
	}
	return stdout.Bytes(), nil
}
