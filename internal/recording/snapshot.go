package recording

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/smazurov/videonode/internal/logging"
	"github.com/smazurov/videonode/internal/streaming"
)

const (
	snapshotTimeout = 5 * time.Second
	ffmpegTimeout   = 5 * time.Second
)

// Snapshot captures a single JPEG frame from a running stream and writes it to disk.
// Files are organized as <baseDir>/<streamID>/<timestamp>.jpg.
// Returns the path relative to baseDir (e.g. "test/20260404_005015.jpg").
func Snapshot(stream *streaming.Stream, baseDir string, timeout time.Duration) (string, error) {
	logger := logging.GetLogger("recording")

	streamDir := filepath.Join(baseDir, stream.ID())
	if err := os.MkdirAll(streamDir, 0o755); err != nil {
		return "", fmt.Errorf("create snapshot dir: %w", err)
	}

	keyframe, err := CaptureKeyframe(stream, timeout)
	if err != nil {
		return "", err
	}

	jpegData, err := decodeToJPEG(keyframe.Data, keyframe.Codec)
	if err != nil {
		return "", err
	}

	filename := time.Now().Format("20060102_150405") + ".jpg"
	absPath := filepath.Join(streamDir, filename)

	if err := os.WriteFile(absPath, jpegData, 0o644); err != nil {
		return "", fmt.Errorf("write snapshot: %w", err)
	}

	relPath := filepath.Join(stream.ID(), filename)
	logger.Debug("Snapshot written", "stream_id", stream.ID(), "path", absPath, "bytes", len(jpegData))
	return relPath, nil
}

// decodeToJPEG pipes Annex B video data through FFmpeg to produce a JPEG image.
func decodeToJPEG(annexB []byte, codec CodecType) ([]byte, error) {
	logger := logging.GetLogger("recording")

	ctx, cancel := context.WithTimeout(context.Background(), ffmpegTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-hide_banner",
		"-f", string(codec),
		"-i", "pipe:0",
		"-frames:v", "1",
		"-f", "mjpeg",
		"-q:v", "2",
		"pipe:1",
	)

	cmd.Stdin = bytes.NewReader(annexB)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		logger.Error("FFmpeg snapshot decode failed", "error", err, "stderr", stderr.String())
		return nil, fmt.Errorf("ffmpeg decode failed: %w: %s", err, stderr.String())
	}

	if stderr.Len() > 0 {
		logger.Debug("FFmpeg snapshot stderr", "output", stderr.String())
	}

	return stdout.Bytes(), nil
}
